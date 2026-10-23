
-- SQL API lives in the selected application schema; existing migration checksums remain unchanged.
CREATE FUNCTION catalog_snapshot(kind text,r jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
 SELECT CASE WHEN kind='policy' THEN jsonb_build_object(
 'id',r->'id','kind',CASE WHEN r->>'database_id' IS NULL THEN 'instance' ELSE 'database' END,
 'objectId',to_jsonb(coalesce(r->>'database_id',r->>'instance_id')),'name',r->'name','enabled',r->'enabled',
 'backupType',r->'backup_type','scheduler',r->'scheduler','schedule',r->'schedule','storage',r->'storage',
 'retentionDays',r->'retention_days','description',r->'description','version',r->'version')
 ELSE jsonb_build_object('id',r->'id','kind',kind,'parentId',
 CASE kind WHEN 'server' THEN '' ELSE coalesce(r->>'instance_id',r->>'server_id','') END,
 'name',r->>'name','address',coalesce(r->>'address',''),'os',coalesce(r->>'os',''),
 'environment',coalesce(r->>'environment',''),'status',r->>'status','description',r->>'description',
 'dbms',coalesce(r->>'dbms',''),'dbmsVersion',coalesce(r->>'dbms_version',''),'port',r->'port',
 'purpose',coalesce(r->>'purpose',''),'version',r->'version',
 'attributes',jsonb_set(coalesce(r->'attributes','{}'),'{administration}',
 CASE WHEN jsonb_typeof(r->'attributes'->'administration')='array' THEN r->'attributes'->'administration' ELSE '[]' END))
 END
$$;
CREATE VIEW v_catalog_objects AS
 SELECT 'server'::text entity,id,NULL::uuid parent_id,name,version,to_jsonb(s)-'search_text' AS row_data FROM servers s
 UNION ALL SELECT 'instance',id,server_id,name,version,to_jsonb(i)-'search_text' FROM instances i
 UNION ALL SELECT 'database',id,coalesce(instance_id,server_id),name,version,to_jsonb(d)-'search_text' FROM databases d;

CREATE VIEW v_database_context AS
 SELECT d.*,s.id AS catalog_server_id,s.name AS server_name,i.name AS instance_name,
 coalesce(nullif(d.dbms,''),i.dbms) AS effective_dbms,
 coalesce(dp.id,ip.id) AS effective_policy_id,coalesce(dp.enabled,ip.enabled) AS policy_enabled,
 concat_ws(' / ',s.name,coalesce(i.name,'Oracle Database'),d.name) AS object_path
 FROM databases d LEFT JOIN instances i ON i.id=d.instance_id
 JOIN servers s ON s.id=coalesce(d.server_id,i.server_id)
 LEFT JOIN backup_policies dp ON dp.database_id=d.id LEFT JOIN backup_policies ip ON ip.instance_id=d.instance_id;

CREATE INDEX favorites_server_target ON favorites(server_id) WHERE server_id IS NOT NULL;
CREATE INDEX favorites_instance_target ON favorites(instance_id) WHERE instance_id IS NOT NULL;
CREATE INDEX favorites_database_target ON favorites(database_id) WHERE database_id IS NOT NULL;
CREATE INDEX databases_parent_catalog_order ON databases(coalesce(instance_id,server_id),lower(name),id);
CREATE INDEX databases_responsible_filter ON databases((attributes->>'responsible'));
CREATE STATISTICS databases_filter_stats(mcv,dependencies) ON environment,status FROM databases;

-- A local document per object avoids duplicating a server's instruction into every descendant.
CREATE TABLE catalog_search_documents(
 entity text NOT NULL,object_id uuid NOT NULL,body text NOT NULL,PRIMARY KEY(entity,object_id)
);
CREATE FUNCTION catalog_sync_search() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE k text;oid uuid;
BEGIN
 IF TG_TABLE_NAME='object_attachments' THEN
  IF TG_OP='DELETE' THEN k:=OLD.entity;oid:=OLD.object_id;ELSE k:=NEW.entity;oid:=NEW.object_id;END IF;
 ELSE
  k:=CASE TG_TABLE_NAME WHEN 'servers' THEN 'server' WHEN 'instances' THEN 'instance' ELSE 'database' END;
  IF TG_OP='DELETE' THEN DELETE FROM catalog_search_documents WHERE entity=k AND object_id=OLD.id;RETURN OLD;END IF;
  oid:=NEW.id;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM v_catalog_objects WHERE entity=k AND id=oid) THEN RETURN NULL; END IF;
 INSERT INTO catalog_search_documents(entity,object_id,body)
 SELECT k,oid,concat_ws(' ',CASE k
 WHEN 'server' THEN (SELECT search_text FROM servers WHERE id=oid)
 WHEN 'instance' THEN (SELECT search_text FROM instances WHERE id=oid)
 ELSE (SELECT search_text FROM databases WHERE id=oid) END,
 (SELECT string_agg(search_text,' ' ORDER BY id) FROM object_attachments WHERE entity=k AND object_id=oid))
 ON CONFLICT(entity,object_id) DO UPDATE SET body=excluded.body;
 RETURN NULL;
END $$;
INSERT INTO catalog_search_documents
 SELECT o.entity,o.id,concat_ws(' ',t.search_text,a.text)
 FROM v_catalog_objects o JOIN(
 SELECT 'server' entity,id,search_text FROM servers UNION ALL SELECT 'instance',id,search_text FROM instances UNION ALL SELECT 'database',id,search_text FROM databases
 )t ON t.entity=o.entity AND t.id=o.id
 LEFT JOIN(SELECT entity,object_id,string_agg(search_text,' ' ORDER BY id)text FROM object_attachments GROUP BY entity,object_id)a ON a.entity=o.entity AND a.object_id=o.id;
CREATE TRIGGER servers_search AFTER INSERT OR UPDATE OR DELETE ON servers FOR EACH ROW EXECUTE FUNCTION catalog_sync_search();
CREATE TRIGGER instances_search AFTER INSERT OR UPDATE OR DELETE ON instances FOR EACH ROW EXECUTE FUNCTION catalog_sync_search();
CREATE TRIGGER databases_search AFTER INSERT OR UPDATE OR DELETE ON databases FOR EACH ROW EXECUTE FUNCTION catalog_sync_search();
CREATE TRIGGER attachments_search AFTER INSERT OR UPDATE OR DELETE ON object_attachments FOR EACH ROW EXECUTE FUNCTION catalog_sync_search();
-- Optional trusted extension: functionality remains correct when installation is unavailable.
DO $$
DECLARE ns text;
BEGIN
 BEGIN
  IF NOT EXISTS(SELECT 1 FROM pg_extension WHERE extname='pg_trgm') THEN CREATE EXTENSION pg_trgm WITH SCHEMA public; END IF;
 EXCEPTION WHEN insufficient_privilege OR undefined_file OR feature_not_supported THEN NULL;
 END;
 SELECT n.nspname INTO ns FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pg_trgm';
 IF ns IS NOT NULL THEN
  EXECUTE format('CREATE INDEX catalog_search_trgm ON catalog_search_documents USING gin(body %I.gin_trgm_ops)',ns);
 END IF;
END $$;

CREATE FUNCTION api_catalog_matches(text[],text,text,text,text,text,boolean) RETURNS TABLE(entity text,id uuid,parent_id uuid) LANGUAGE sql STABLE AS $$
WITH simple_terms AS MATERIALIZED(SELECT DISTINCT lower(t) term FROM unnest($1::text[])t WHERE strpos(t,' ')=0),
 hits AS MATERIALIZED(SELECT t.term,x.entity,x.object_id FROM simple_terms t JOIN catalog_search_documents x
 ON x.body LIKE '%'||replace(replace(replace(t.term,'\','\\'),'%','\%'),'_','\_')||'%' ESCAPE '\'),
 candidate_pairs AS(
 SELECT h.term,d.id FROM hits h JOIN databases d ON h.entity='database' AND d.id=h.object_id
 UNION SELECT h.term,d.id FROM hits h JOIN databases d ON h.entity='instance' AND d.instance_id=h.object_id
 UNION SELECT h.term,d.id FROM hits h JOIN instances i ON h.entity='server' AND i.server_id=h.object_id JOIN databases d ON d.instance_id=i.id
 UNION SELECT h.term,d.id FROM hits h JOIN databases d ON h.entity='server' AND d.server_id=h.object_id
 ),candidates AS MATERIALIZED(SELECT id FROM candidate_pairs GROUP BY id HAVING count(*)=(SELECT count(*) FROM simple_terms)),
 attachment_search AS MATERIALIZED(SELECT entity,object_id,string_agg(search_text,' ') AS search_text FROM object_attachments WHERE cardinality($1::text[])>0 GROUP BY entity,object_id),eligible_db AS MATERIALIZED(
 SELECT d.id,d.instance_id,d.server_id FROM databases d
 LEFT JOIN instances i ON i.id=d.instance_id JOIN servers s ON s.id=coalesce(d.server_id,i.server_id)
 LEFT JOIN attachment_search sa ON sa.entity='server' AND sa.object_id=s.id
 LEFT JOIN attachment_search ia ON ia.entity='instance' AND ia.object_id=i.id
 LEFT JOIN attachment_search da ON da.entity='database' AND da.object_id=d.id
 LEFT JOIN backup_policies dp ON dp.database_id=d.id LEFT JOIN backup_policies ip ON ip.instance_id=d.instance_id
 WHERE (NOT EXISTS(SELECT 1 FROM simple_terms) OR d.id IN(SELECT id FROM candidates)) AND ($2='' OR coalesce(nullif(d.dbms,''),i.dbms)=$2) AND($3='' OR d.status=$3) AND($4='' OR d.environment=$4)
 AND($5='' OR CASE WHEN $5='__empty__' THEN trim(coalesce(d.attributes->>'responsible',''))='' ELSE d.attributes->>'responsible'=$5 END)
 AND($6 IN('','any') OR($6='present' AND coalesce(dp.id,ip.id) IS NOT NULL) OR($6='missing' AND coalesce(dp.id,ip.id) IS NULL)
 OR($6='active' AND coalesce(dp.enabled,ip.enabled)) OR($6='disabled' AND NOT coalesce(dp.enabled,ip.enabled,true)))
 AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) term WHERE strpos(concat_ws(' ',s.search_text,sa.search_text,i.search_text,ia.search_text,d.search_text,da.search_text),lower(term))=0)
),eligible_instance AS MATERIALIZED(
 SELECT i.id,i.server_id FROM instances i JOIN servers s ON s.id=i.server_id
 LEFT JOIN attachment_search sa ON sa.entity='server' AND sa.object_id=s.id
 LEFT JOIN attachment_search ia ON ia.entity='instance' AND ia.object_id=i.id
 WHERE EXISTS(SELECT 1 FROM eligible_db d WHERE d.instance_id=i.id)
 OR($7 AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) term WHERE strpos(concat_ws(' ',s.search_text,sa.search_text,i.search_text,ia.search_text),lower(term))=0))
),eligible_server AS MATERIALIZED(
 SELECT s.id FROM servers s LEFT JOIN attachment_search sa ON sa.entity='server' AND sa.object_id=s.id WHERE EXISTS(SELECT 1 FROM eligible_instance i WHERE i.server_id=s.id)
 OR EXISTS(SELECT 1 FROM eligible_db d WHERE d.server_id=s.id)
 OR($7 AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) term WHERE strpos(concat_ws(' ',s.search_text,sa.search_text),lower(term))=0))
)  SELECT 'server',s.id,NULL::uuid FROM eligible_server s
 UNION ALL SELECT 'instance',i.id,i.server_id FROM eligible_instance i
 UNION ALL SELECT 'database',d.id,coalesce(d.instance_id,d.server_id) FROM eligible_db d $$;

CREATE FUNCTION api_catalog_page(f jsonb,scope text,parent uuid,requested integer) RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE terms text[];m jsonb;kind text;lim int;total bigint;pg int;counts jsonb;items jsonb;children jsonb;plain boolean;
BEGIN
 IF scope NOT IN('','server','instance','oracle') OR length(coalesce(f->>'query',''))>2000 THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Invalid catalog request';END IF;
 IF coalesce(f->>'policy','') NOT IN('','any','active','disabled','present','missing') THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Invalid policy filter';END IF;
 SELECT coalesce(array_agg(DISTINCT btrim(t)) FILTER(WHERE btrim(t)<>''),'{}') INTO terms FROM unnest(string_to_array(coalesce(f->>'query',''),'|'))t;
 plain:=cardinality(terms)=0 AND coalesce(f->>'dbms','')='' AND coalesce(f->>'status','')='' AND coalesce(f->>'environment','')='' AND coalesce(f->>'responsible','')='' AND coalesce(f->>'policy','') IN('','any');
 kind:=CASE scope WHEN '' THEN 'server' WHEN 'server' THEN 'instance' ELSE 'database' END;
 lim:=CASE scope WHEN '' THEN 10 ELSE 100 END;
 IF plain THEN
  SELECT jsonb_build_object('servers',(SELECT count(*) FROM servers),'instances',(SELECT count(*) FROM instances),'databases',(SELECT count(*) FROM databases)) INTO counts;
  IF scope='' THEN total:=(counts->>'servers')::bigint;
  ELSIF scope='server' THEN SELECT count(*) INTO total FROM instances WHERE server_id=parent;
  ELSIF scope='instance' THEN SELECT count(*) INTO total FROM databases WHERE instance_id=parent;
  ELSE SELECT count(*) INTO total FROM databases WHERE server_id=parent; END IF;
  pg:=greatest(0,least(greatest(0,requested),greatest(0,(total-1)/lim)::int));
  SELECT coalesce(jsonb_agg(catalog_snapshot(kind,row_data) ORDER BY lower(name),id),'[]') INTO items FROM(
   SELECT * FROM v_catalog_objects WHERE entity=kind AND(scope='' OR parent_id=parent)
   AND(scope<>'oracle' OR row_data->>'dbms'='Oracle Database') AND(scope<>'instance' OR row_data->>'dbms'='')
   ORDER BY lower(name),id LIMIT lim OFFSET pg*lim
  )p;
  SELECT coalesce(jsonb_object_agg(id::text,jsonb_build_object('instances',ic,'databases',dc,'oracle',oc)),'{}') INTO children FROM(
   SELECT (x->>'id')::uuid id,
   CASE WHEN kind='server' THEN(SELECT count(*) FROM instances WHERE server_id=(x->>'id')::uuid) ELSE 0 END ic,
   CASE WHEN kind='instance' THEN(SELECT count(*) FROM databases WHERE instance_id=(x->>'id')::uuid) ELSE 0 END dc,
   CASE WHEN kind='server' THEN(SELECT count(*) FROM databases WHERE server_id=(x->>'id')::uuid) ELSE 0 END oc
   FROM jsonb_array_elements(items)x WHERE kind<>'database'
  )c;
 ELSE
  WITH matched AS MATERIALIZED(SELECT * FROM api_catalog_matches(terms,coalesce(f->>'dbms',''),coalesce(f->>'status',''),coalesce(f->>'environment',''),coalesce(f->>'responsible',''),coalesce(f->>'policy',''),
   coalesce(f->>'dbms','')='' AND coalesce(f->>'status','')='' AND coalesce(f->>'environment','')='' AND coalesce(f->>'responsible','')='' AND coalesce(f->>'policy','') IN('','any'))),
  scoped AS MATERIALIZED(SELECT o.* FROM v_catalog_objects o JOIN matched e ON e.entity=o.entity AND e.id=o.id
    WHERE o.entity=kind AND(scope='' OR o.parent_id=parent)
    AND(scope<>'oracle' OR o.row_data->>'dbms'='Oracle Database') AND(scope<>'instance' OR o.row_data->>'dbms'='')),
  sz AS(SELECT count(*) n FROM scoped),page AS(SELECT greatest(0,least(greatest(0,requested),greatest(0,(n-1)/lim)::int))p FROM sz),
  selected AS MATERIALIZED(SELECT * FROM scoped ORDER BY lower(name),id LIMIT lim OFFSET(SELECT p*lim FROM page))
  SELECT jsonb_build_object(
   'counts',jsonb_build_object('servers',count(*) FILTER(WHERE entity='server'),'instances',count(*) FILTER(WHERE entity='instance'),'databases',count(*) FILTER(WHERE entity='database')),
   'total',(SELECT n FROM sz),'page',(SELECT p FROM page),
   'items',(SELECT coalesce(jsonb_agg(catalog_snapshot(kind,row_data) ORDER BY lower(name),id),'[]') FROM selected),
   'children',(SELECT coalesce(jsonb_object_agg(s.id::text,jsonb_build_object(
     'instances',(SELECT count(*) FROM matched e WHERE e.entity='instance' AND e.parent_id=s.id),
     'databases',(SELECT count(*) FROM matched e WHERE e.entity='database' AND e.parent_id=s.id AND kind='instance'),
     'oracle',(SELECT count(*) FROM matched e WHERE e.entity='database' AND e.parent_id=s.id AND kind='server'))),'{}') FROM selected s WHERE kind<>'database'))
  INTO m FROM matched;
  RETURN m||jsonb_build_object('pageSize',lim);
 END IF;
 RETURN jsonb_build_object('counts',counts,'total',total,'page',pg,'pageSize',lim,'items',items,'children',children);
END $$;
-- Qualified DML from maintenance tools must resolve triggers inside their owning schema.
DO $$
DECLARE f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('catalog_sync_search','api_catalog_page','api_catalog_matches')
 LOOP EXECUTE format('ALTER FUNCTION %s SET search_path TO %I,pg_catalog,pg_temp',f.signature,current_schema()); END LOOP;
END $$;
