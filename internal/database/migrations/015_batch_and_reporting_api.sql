CREATE FUNCTION api_apply_staged_writes() RETURNS integer LANGUAGE plpgsql AS $$
DECLARE r record;n int:=0;
BEGIN
 PERFORM catalog_actor(false);
 FOR r IN SELECT * FROM pg_temp.catalog_write_stage ORDER BY seq LOOP
  PERFORM api_save_record(r.kind,r.payload,r.expected,r.mode);
  n:=n+1;
 END LOOP;
 RETURN n;
END $$;
CREATE FUNCTION api_bulk_update(changes jsonb) RETURNS integer LANGUAGE plpgsql AS $$
DECLARE uid uuid;n int;
BEGIN
 uid:=catalog_actor(false);
 IF jsonb_array_length(changes)>10000 THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Too many rows';END IF;
 UPDATE databases d SET attributes=x.r->'attributes',status=x.r->>'status',updated_by=uid
 FROM jsonb_array_elements(changes)x(r)
 WHERE d.id=(x.r->>'id')::uuid AND d.version=(x.r->>'version')::bigint;
 GET DIAGNOSTICS n=ROW_COUNT;
 IF n<>jsonb_array_length(changes) THEN RAISE EXCEPTION USING ERRCODE='CD014',MESSAGE='Bulk selection changed';END IF;
 RETURN n;
END $$;
CREATE FUNCTION api_quality_issues() RETURNS TABLE(code text,severity text,kind text,id uuid,name text,path text,detail text) LANGUAGE sql STABLE AS $$
 WITH db AS MATERIALIZED(
 SELECT id,instance_id,server_id,name,environment,attributes,object_path,policy_enabled,
 count(*)OVER(PARTITION BY coalesce(instance_id,server_id),lower(name)) similar_count FROM v_database_context
 )
 SELECT 'missing_responsible','warning','database',id,name,object_path,'Не указано ответственное лицо' FROM db WHERE btrim(coalesce(attributes->>'responsible',''))=''
 UNION ALL SELECT 'missing_address','warning','server',id,name,name,'Не указан сетевой адрес сервера' FROM servers WHERE btrim(address)=''
 UNION ALL SELECT 'production_no_policy','danger','database',id,name,object_path,'Промышленная БД без политики РК' FROM db WHERE environment='Промышленный' AND policy_enabled IS NULL
 UNION ALL SELECT 'production_disabled_policy','warning','database',id,name,object_path,'Эффективная политика РК промышленной БД отключена' FROM db WHERE environment='Промышленный' AND policy_enabled=false
 UNION ALL SELECT 'duplicate_server_name','warning','server',id,name,concat_ws(' • ',name,address),'Имена серверов совпадают без учёта регистра и крайних пробелов' FROM servers WHERE lower(btrim(name)) IN(SELECT lower(btrim(name)) FROM servers GROUP BY lower(btrim(name)) HAVING count(*)>1)
 UNION ALL SELECT 'duplicate_server_address','warning','server',id,name,concat_ws(' • ',name,address),'Один адрес у нескольких серверов: возможны допустимые псевдонимы' FROM servers WHERE btrim(address)<>'' AND lower(btrim(address)) IN(SELECT lower(btrim(address)) FROM servers WHERE btrim(address)<>'' GROUP BY lower(btrim(address)) HAVING count(*)>1)
 UNION ALL SELECT 'similar_sid','warning','database',id,name,object_path,'SID различаются только регистром внутри одного родителя; проверьте допустимость' FROM db WHERE similar_count>1
$$;
CREATE FUNCTION api_lookups() RETURNS jsonb LANGUAGE sql STABLE AS $$
 WITH suggestions AS(
 SELECT DISTINCT category,value FROM(
 SELECT attributes FROM servers UNION ALL SELECT attributes FROM instances UNION ALL SELECT attributes FROM databases
 )o CROSS JOIN LATERAL(VALUES('location',btrim(attributes->>'location')),('serviceOrganization',btrim(attributes->>'serviceOrganization')),('supervisor',btrim(attributes->>'supervisor')),('responsible',btrim(attributes->>'responsible')))v(category,value) WHERE length(value)>0
 ),values AS(
 SELECT category,value,position,false extra FROM lookup_values
 UNION ALL SELECT s.category,s.value,0,true FROM suggestions s WHERE NOT EXISTS(SELECT 1 FROM lookup_values l WHERE l.category=s.category AND l.value=s.value)
 ),groups AS(SELECT category,jsonb_agg(value ORDER BY extra,position,value COLLATE "C")items FROM values GROUP BY category)
 SELECT jsonb_build_object('location','[]'::jsonb,'serviceOrganization','[]'::jsonb,'supervisor','[]'::jsonb,'responsible','[]'::jsonb)||coalesce(jsonb_object_agg(category,items),'{}') FROM groups
$$;
CREATE FUNCTION api_statistics() RETURNS jsonb LANGUAGE sql STABLE AS $$
 WITH db AS MATERIALIZED(SELECT effective_dbms,environment,status,effective_policy_id,policy_enabled FROM v_database_context),
 dbms AS(
 SELECT dbms,sum(instances)::bigint instances,sum(databases)::bigint databases FROM(
 SELECT i.dbms,count(DISTINCT i.id)instances,count(d.id)databases FROM instances i LEFT JOIN databases d ON d.instance_id=i.id GROUP BY i.dbms
 UNION ALL SELECT dbms,0,count(*) FROM databases WHERE server_id IS NOT NULL GROUP BY dbms
 )x GROUP BY dbms)
 SELECT jsonb_build_object(
 'servers',(SELECT count(*) FROM servers),'uniqueServers',(SELECT count(DISTINCT lower(btrim(name))) FROM servers),
 'serverAddresses',(SELECT count(DISTINCT(lower(btrim(name)),lower(btrim(address)))) FROM servers),
 'instances',(SELECT count(*) FROM instances),'databases',(SELECT count(*) FROM db),
 'policies',(SELECT count(*) FROM backup_policies),'enabledPolicies',(SELECT count(*) FROM backup_policies WHERE enabled),
 'protectedDatabases',(SELECT count(*) FROM db WHERE policy_enabled),'unprotectedDatabases',(SELECT count(*) FROM db WHERE effective_policy_id IS NULL),
 'disabledDatabases',(SELECT count(*) FROM db WHERE policy_enabled=false),
 'dbms',(SELECT coalesce(jsonb_agg(jsonb_build_object('name',dbms,'instances',instances,'databases',databases)ORDER BY databases DESC,dbms),'[]')FROM dbms),
 'statuses',(SELECT coalesce(jsonb_agg(jsonb_build_object('name',status,'instances',0,'databases',n)ORDER BY n DESC,status),'[]')FROM(SELECT status,count(*)n FROM db GROUP BY status)x),
 'environments',(SELECT coalesce(jsonb_agg(jsonb_build_object('name',environment,'instances',0,'databases',n)ORDER BY n DESC,environment),'[]')FROM(SELECT environment,count(*)n FROM db GROUP BY environment)x),
 'generatedAt',statement_timestamp())
$$;
DO $$ DECLARE f record;BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('api_apply_staged_writes','api_bulk_update','api_quality_issues','api_lookups','api_statistics')
 LOOP EXECUTE format('ALTER FUNCTION %s SET search_path TO %I,pg_catalog,pg_temp',f.signature,current_schema());END LOOP;
END $$;
