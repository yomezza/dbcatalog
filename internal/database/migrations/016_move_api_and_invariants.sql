CREATE FUNCTION api_move_preview(r jsonb) RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE k text;parentkind text;b jsonb;dest jsonb;a jsonb;afterrow jsonb;out jsonb;uid uuid;conflicts jsonb:='[]';fromdbms text;todbms text;fingerprint text;
BEGIN
 uid:=catalog_actor(false);k:=r->>'kind';
 IF k NOT IN('instance','database') OR length(btrim(coalesce(r->>'reason','')))=0 OR octet_length(r->>'reason')>2000 THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Invalid move request';END IF;
 SELECT row_data INTO b FROM v_catalog_objects WHERE entity=k AND id=(r->>'id')::uuid;
 IF b IS NULL THEN RAISE EXCEPTION USING ERRCODE='CD013',MESSAGE='Move source missing';END IF;
 IF(b->>'version')::bigint<>(r->>'version')::bigint THEN RAISE EXCEPTION USING ERRCODE='CD014',MESSAGE='Source changed';END IF;
 parentkind:=CASE WHEN k='instance' OR b->>'dbms'='Oracle Database' THEN 'server' ELSE 'instance' END;
 SELECT row_data INTO dest FROM v_catalog_objects WHERE entity=parentkind AND id=(r->>'parentId')::uuid;
 IF dest IS NULL THEN RAISE EXCEPTION USING ERRCODE='CD013',MESSAGE='Destination missing';END IF;
 IF(dest->>'version')::bigint<>(r->>'parentVersion')::bigint THEN RAISE EXCEPTION USING ERRCODE='CD014',MESSAGE='Destination changed';END IF;
 IF coalesce(b->>'instance_id',b->>'server_id')=r->>'parentId' THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Same parent';END IF;
 afterrow:=b||jsonb_build_object(CASE WHEN parentkind='server' THEN 'server_id' ELSE 'instance_id' END,r->>'parentId');
 a:=catalog_snapshot(k,afterrow);
 fromdbms:=b->>'dbms';todbms:=fromdbms;
 IF k='database' AND coalesce(b->>'dbms','')='' THEN
  SELECT dbms INTO fromdbms FROM instances WHERE id=(b->>'instance_id')::uuid;
  todbms:=dest->>'dbms';
 END IF;
 out:=jsonb_build_object('object',catalog_snapshot(k,b),'destination',catalog_snapshot(parentkind,dest),
 'fromDBMS',coalesce(fromdbms,''),'toDBMS',coalesce(todbms,''),'fromPath',catalog_object_path(k,b),'toPath',catalog_object_path(k,afterrow),
 'fromPolicy',catalog_object_policy(k,b),'toPolicy',catalog_object_policy(k,afterrow),
 'databases',CASE WHEN k='instance' THEN(SELECT count(*) FROM databases WHERE instance_id=(r->>'id')::uuid) ELSE 0 END,
 'attachments',(SELECT count(*) FROM object_attachments WHERE(entity=k AND object_id=(r->>'id')::uuid)OR(k='instance' AND entity='database' AND object_id IN(SELECT id FROM databases WHERE instance_id=(r->>'id')::uuid))));
 fingerprint:=encode(sha256(convert_to(jsonb_build_object('schema',current_schema(),'user',uid,'request',r,'preview',out)::text,'UTF8')),'hex');
 -- Validate the exact write in a subtransaction that is always rolled back.
 BEGIN
  PERFORM api_save_record(k,a,(r->>'version')::bigint,0,r->>'reason');
  RAISE EXCEPTION USING ERRCODE='CD099',MESSAGE='Preview rollback';
 EXCEPTION WHEN SQLSTATE 'CD099' THEN NULL;
 WHEN serialization_failure OR deadlock_detected THEN RAISE;
 WHEN OTHERS THEN conflicts:=jsonb_build_array(CASE SQLSTATE WHEN '23505' THEN 'Запись с таким именем уже существует у нового родителя' WHEN '23503' THEN 'Родитель отсутствует или изменился' WHEN 'CD011' THEN 'Значение отсутствует в справочнике' ELSE 'Перемещение нарушает ограничения записи' END);
 END;
 RETURN out||jsonb_build_object('conflicts',conflicts,'fingerprint',fingerprint);
END $$;
CREATE FUNCTION api_move_object(r jsonb,fingerprint text) RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE preview jsonb;after_object jsonb;
BEGIN
 PERFORM catalog_actor(false);
 LOCK TABLE servers,instances,databases,backup_policies IN SHARE ROW EXCLUSIVE MODE;
 preview:=api_move_preview(r);
 IF fingerprint IS NULL OR fingerprint='' OR preview->>'fingerprint'<>fingerprint OR jsonb_array_length(preview->'conflicts')>0 THEN RAISE EXCEPTION USING ERRCODE='CD014',MESSAGE='Move preview changed';END IF;
 after_object:=preview->'object'||jsonb_build_object('parentId',r->>'parentId');
 RETURN api_save_record(r->>'kind',after_object,(r->>'version')::bigint,0,r->>'reason');
END $$;
DO $$ DECLARE f record;BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('api_move_preview','api_move_object')
 LOOP EXECUTE format('ALTER FUNCTION %s SET search_path TO %I,pg_catalog,pg_temp',f.signature,current_schema());END LOOP;
END $$;
-- Reject already-connected old desktop writers too; this is compatibility, not authentication.
CREATE FUNCTION catalog_guard_client() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF current_setting('application_name')='CatalogDB' AND current_setting('catalogdb.protocol',true) IS DISTINCT FROM '2' THEN
  RAISE EXCEPTION USING ERRCODE='CD016',MESSAGE='Desktop client upgrade required';
 END IF;
 RETURN NULL;
END $$;
DO $$ DECLARE tbl text;BEGIN
 FOREACH tbl IN ARRAY ARRAY['servers','instances','databases','backup_policies','users','audit_events','object_history','object_attachments','favorites','saved_filters','notifications','deleted_objects'] LOOP
  EXECUTE format('CREATE TRIGGER catalog_protocol BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH STATEMENT EXECUTE FUNCTION catalog_guard_client()',tbl);
 END LOOP;
END $$;
