-- CatalogDB: transactional upgrade from schema 009. Existing rows and history are preserved.
-- Run with psql -X -v catalog_schema=YOUR_SCHEMA -f THIS_FILE against a backed-up database.
-- No old migration is modified. The application also embeds and applies these migrations.
\set ON_ERROR_STOP on
\if :{?catalog_schema}
\else
\set catalog_schema dbcatalog
\endif
BEGIN;
SET LOCAL statement_timeout='10min';
SET LOCAL lock_timeout='30s';
SET LOCAL catalogdb.protocol='2';
SELECT pg_advisory_xact_lock(724194301112);
SELECT set_config('catalogdb.migration_schema',:'catalog_schema',true);
SELECT set_config('search_path',format('%I,pg_catalog,pg_temp',:'catalog_schema'),true);
DO $catalog_check$ BEGIN
IF current_schema() IS DISTINCT FROM current_setting('catalogdb.migration_schema') OR to_regclass('schema_migrations') IS NULL THEN RAISE EXCEPTION 'The existing application schema is required'; END IF;
IF EXISTS(SELECT 1 FROM schema_migrations m LEFT JOIN(VALUES
(1,'dc2cd5a4bfb2519eb25068898e9fedaf9095e8fa732d42f739267b0145dda8f9'),
(2,'2f848e03efe83326ed7c81ed8f3be6d5d2ad8badfeb17d8849c3a48163aed0e6'),
(3,'779ea9efc122ccce4a0d356eb06de3f00c71da0b75f89253dbd00312bf8235fd'),
(4,'6237844b51398bd876f352f20c8d86a0d6617d7e8169e396cbaa8e33856bcf2d'),
(5,'bee19feca2309e81822f4a8976d20d6bcc680840807c86d1d2b3f4751c519bc1'),
(6,'79942fdeaa58be59a1fc9413dd94fd625f37ad26822f88e7bdf49a2ec17c8fb9'),
(7,'ced3d451e44e275a95407cc977d13cf91dd3a1c0bb67b5183d961b7fcba7dd4e'),
(8,'aed44a806ed29dc14f7bd1e2a46cb85fb998cf030ff883505c7ba23f3ade5640'),
(9,'df66ee6e0c9e169fff98b616ace49b5752201452404aa40fc3dc05e7f1edf9f8'),
(10,'8660aa071f7b6719c785a32f7c131578b28da292d5a8bce2c6798bc2c8dbc963'),
(11,'9819caf277a1b2dbccc3340d45e2dbc7833c265ca65b80e110088e4ab030f395'),
(12,'c5741281ed6fdac9239692d250264bb1eb17ccced0157234b5830e456a6f91ad'),
(13,'eabc713b9003caca20ac3430d0c88ba459a8438f6e3cd9341efeb04408e14356'),
(14,'772727396e6421a82c68353cfb4392b3f00eae0d59b3f52af2937f1e3111491a'),
(15,'d863d1537ca500adfb5352cff85fdde2ce99e7d6612d9d7de546524f59a45433'),
(16,'813ab061a35d72b5dfadca1fb3a9478717301cc1e7732aaa73fc124af65407d8')
)e(version,checksum)ON e.version=m.version WHERE e.version IS NULL OR e.checksum<>m.checksum) THEN RAISE EXCEPTION 'Unknown migration or checksum mismatch';END IF;
IF(SELECT count(*) FROM schema_migrations WHERE version BETWEEN 1 AND 9)<>9 THEN RAISE EXCEPTION 'Apply original migrations 001-009 first';END IF;
IF EXISTS(SELECT 1 FROM generate_series(1,(SELECT max(version)::int FROM schema_migrations))g WHERE NOT EXISTS(SELECT 1 FROM schema_migrations m WHERE m.version=g)) THEN RAISE EXCEPTION 'Migration ledger contains a gap';END IF;
END $catalog_check$;
-- 010_catalog_read_api.sql
DO $catalog_step$ BEGIN
IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=10) THEN
EXECUTE $catalog_source$

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

$catalog_source$;
INSERT INTO schema_migrations(version,name,checksum)VALUES(10,'010_catalog_read_api.sql','8660aa071f7b6719c785a32f7c131578b28da292d5a8bce2c6798bc2c8dbc963');
END IF; END $catalog_step$;
-- 011_partitioned_audit.sql
DO $catalog_step$ BEGIN
IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=11) THEN
EXECUTE $catalog_source$
LOCK TABLE audit_events IN ACCESS EXCLUSIVE MODE;
ALTER TABLE audit_events RENAME TO audit_events_legacy;
CREATE TABLE audit_events(
 LIKE audit_events_legacy INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING IDENTITY,
 PRIMARY KEY(created_at,id),
 FOREIGN KEY(actor_id) REFERENCES users(id)
) PARTITION BY RANGE(created_at);
CREATE INDEX audit_created_v2 ON audit_events(created_at DESC,id DESC);
CREATE INDEX audit_actor_v2 ON audit_events(actor_id,created_at DESC,id DESC);
CREATE INDEX audit_object_v2 ON audit_events(entity,object_id,id DESC);
CREATE TABLE audit_events_default PARTITION OF audit_events DEFAULT;

-- Called by existing application maintenance, not by a row-insert trigger.
-- No SECURITY DEFINER: DDL still requires the operator's real PostgreSQL privileges.
CREATE FUNCTION maintain_catalog_partitions() RETURNS integer LANGUAGE plpgsql AS $$
DECLARE lo timestamptz;hi timestamptz;part text;n integer:=0;
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(current_schema()||':audit-partitions',0));
 FOR lo IN SELECT DISTINCT d FROM(
  SELECT date_trunc('month',created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' d FROM audit_events_default
  UNION SELECT(date_trunc('month',now() AT TIME ZONE 'UTC')+make_interval(months=>g)) AT TIME ZONE 'UTC' FROM generate_series(0,3)g
 )x ORDER BY d
 LOOP
  hi:=((lo AT TIME ZONE 'UTC')+interval '1 month') AT TIME ZONE 'UTC';
  part:='audit_events_'||to_char(lo AT TIME ZONE 'UTC','YYYYMM');
  IF to_regclass(format('%I.%I',current_schema(),part)) IS NOT NULL THEN CONTINUE; END IF;
  EXECUTE 'LOCK TABLE audit_events_default IN ACCESS EXCLUSIVE MODE';
  EXECUTE format('CREATE TABLE %I (LIKE audit_events INCLUDING DEFAULTS INCLUDING CONSTRAINTS)',part);
  EXECUTE format('WITH moved AS(DELETE FROM audit_events_default WHERE created_at >= $1 AND created_at < $2 RETURNING *) INSERT INTO %I SELECT * FROM moved',part) USING lo,hi;
  EXECUTE format('ALTER TABLE audit_events ATTACH PARTITION %I FOR VALUES FROM(%L) TO(%L)',part,lo,hi);
  n:=n+1;
 END LOOP;
 RETURN n;
END $$;
DO $$ BEGIN EXECUTE format('ALTER FUNCTION maintain_catalog_partitions() SET search_path TO %I,pg_catalog,pg_temp',current_schema());END $$;
INSERT INTO audit_events OVERRIDING SYSTEM VALUE SELECT * FROM audit_events_legacy;
SELECT setval(pg_get_serial_sequence('audit_events','id'),greatest(coalesce((SELECT max(id) FROM audit_events),0),1),EXISTS(SELECT 1 FROM audit_events));
SELECT maintain_catalog_partitions();
DO $$ BEGIN
 IF (SELECT count(*) FROM audit_events)<>(SELECT count(*) FROM audit_events_legacy) THEN RAISE EXCEPTION 'Audit migration row count mismatch';END IF;
 IF EXISTS(SELECT * FROM audit_events_legacy EXCEPT SELECT * FROM audit_events) THEN RAISE EXCEPTION 'Audit migration data mismatch'; END IF;
END $$;
DROP TABLE audit_events_legacy;

$catalog_source$;
INSERT INTO schema_migrations(version,name,checksum)VALUES(11,'011_partitioned_audit.sql','9819caf277a1b2dbccc3340d45e2dbc7833c265ca65b80e110088e4ab030f395');
END IF; END $catalog_step$;
-- 012_server_writes_and_history.sql
DO $catalog_step$ BEGIN
IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=12) THEN
EXECUTE $catalog_source$
LOCK TABLE servers,instances,databases,backup_policies,object_history,record_versions IN SHARE ROW EXCLUSIVE MODE;

-- Canonical versions are hash-partitioned by object; the complete key remains unique.
ALTER TABLE record_versions RENAME TO record_versions_legacy;
CREATE TABLE record_versions(
 LIKE record_versions_legacy INCLUDING DEFAULTS INCLUDING CONSTRAINTS,
 PRIMARY KEY(entity,object_id,version),FOREIGN KEY(actor_id) REFERENCES users(id)
) PARTITION BY HASH(entity,object_id);
DO $$ BEGIN FOR n IN 0..7 LOOP EXECUTE format('CREATE TABLE record_versions_h%s PARTITION OF record_versions FOR VALUES WITH(MODULUS 8,REMAINDER %s)',n,n);END LOOP;END $$;
INSERT INTO record_versions SELECT * FROM record_versions_legacy;
ALTER TABLE object_history RENAME TO object_history_legacy;
CREATE TABLE history_events(
 id bigint GENERATED BY DEFAULT AS IDENTITY,entity text NOT NULL,object_id uuid NOT NULL,
 action text NOT NULL,actor_id uuid NOT NULL REFERENCES users(id),
 before_version bigint,after_version bigint,before_fallback jsonb,after_fallback jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(entity,object_id,id)
) PARTITION BY HASH(entity,object_id);
DO $$ BEGIN FOR n IN 0..7 LOOP EXECUTE format('CREATE TABLE history_events_h%s PARTITION OF history_events FOR VALUES WITH(MODULUS 8,REMAINDER %s)',n,n);END LOOP;END $$;
INSERT INTO history_events(id,entity,object_id,action,actor_id,before_version,after_version,before_fallback,after_fallback,created_at)
 SELECT h.id,h.entity,h.object_id,h.action,h.actor_id,
 CASE WHEN b.snapshot=h.before_state THEN b.version END,CASE WHEN a.snapshot=h.after_state THEN a.version END,
 CASE WHEN b.snapshot=h.before_state THEN NULL ELSE h.before_state END,
 CASE WHEN a.snapshot=h.after_state THEN NULL ELSE h.after_state END,h.created_at
 FROM object_history_legacy h
 LEFT JOIN record_versions b ON b.entity=h.entity AND b.object_id=h.object_id AND b.version=CASE WHEN h.before_state->>'version' ~ '^[1-9][0-9]*$' THEN(h.before_state->>'version')::bigint END
 LEFT JOIN record_versions a ON a.entity=h.entity AND a.object_id=h.object_id AND a.version=CASE WHEN h.after_state->>'version' ~ '^[1-9][0-9]*$' THEN(h.after_state->>'version')::bigint END;
SELECT setval(pg_get_serial_sequence('history_events','id'),greatest(coalesce((SELECT max(id) FROM history_events),0),1),EXISTS(SELECT 1 FROM history_events));
CREATE VIEW object_history AS
 SELECT h.id,h.entity,h.object_id,h.action,h.actor_id,
 coalesce(b.snapshot,h.before_fallback) before_state,coalesce(a.snapshot,h.after_fallback) after_state,h.created_at
 FROM history_events h LEFT JOIN record_versions b ON b.entity=h.entity AND b.object_id=h.object_id AND b.version=h.before_version
 LEFT JOIN record_versions a ON a.entity=h.entity AND a.object_id=h.object_id AND a.version=h.after_version;
DO $$ BEGIN
 IF EXISTS((SELECT * FROM object_history_legacy EXCEPT SELECT * FROM object_history) UNION ALL(SELECT * FROM object_history EXCEPT SELECT * FROM object_history_legacy)) THEN RAISE EXCEPTION 'History migration mismatch'; END IF;
 IF EXISTS((SELECT * FROM record_versions_legacy EXCEPT SELECT * FROM record_versions) UNION ALL(SELECT * FROM record_versions EXCEPT SELECT * FROM record_versions_legacy)) THEN RAISE EXCEPTION 'Versions migration mismatch';END IF;
END $$;
DROP TABLE object_history_legacy;
DROP TABLE record_versions_legacy;
DROP FUNCTION capture_record_version();
ALTER TABLE history_events ADD CONSTRAINT history_before_version FOREIGN KEY(entity,object_id,before_version) REFERENCES record_versions(entity,object_id,version);
ALTER TABLE history_events ADD CONSTRAINT history_after_version FOREIGN KEY(entity,object_id,after_version) REFERENCES record_versions(entity,object_id,version);
CREATE FUNCTION catalog_protect_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='CD015',MESSAGE='Recorded versions are immutable';END $$;
CREATE TRIGGER versions_immutable BEFORE UPDATE OR DELETE ON record_versions FOR EACH ROW EXECUTE FUNCTION catalog_protect_version();


-- Compatibility inserts still work; ordinary object writes now reach this through DB triggers.
CREATE FUNCTION catalog_insert_history() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE bv bigint;av bigint;bs jsonb;asnap jsonb;
BEGIN
 IF NEW.after_state->>'version' ~ '^[1-9][0-9]*$' AND NEW.entity IN('server','instance','database','policy') THEN
  av:=(NEW.after_state->>'version')::bigint;
  INSERT INTO record_versions(entity,object_id,version,snapshot,actor_id,action,created_at)
   VALUES(NEW.entity,NEW.object_id,av,NEW.after_state,NEW.actor_id,NEW.action,coalesce(NEW.created_at,now()));
 END IF;
 IF NEW.before_state->>'version' ~ '^[1-9][0-9]*$' THEN
  SELECT version,snapshot INTO bv,bs FROM record_versions WHERE entity=NEW.entity AND object_id=NEW.object_id AND version=(NEW.before_state->>'version')::bigint;
 END IF;
 IF bs IS DISTINCT FROM NEW.before_state THEN bv:=NULL; END IF;
 INSERT INTO history_events(entity,object_id,action,actor_id,before_version,after_version,before_fallback,after_fallback,created_at)
 VALUES(NEW.entity,NEW.object_id,NEW.action,NEW.actor_id,bv,av,
 CASE WHEN bv IS NULL THEN NEW.before_state END,CASE WHEN av IS NULL THEN NEW.after_state END,coalesce(NEW.created_at,now())) RETURNING id,created_at INTO NEW.id,NEW.created_at;
 RETURN NEW;
END $$;
CREATE TRIGGER history_compat_insert INSTEAD OF INSERT ON object_history FOR EACH ROW EXECUTE FUNCTION catalog_insert_history();

CREATE FUNCTION catalog_actor(needs_admin boolean DEFAULT false) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE uid uuid;ok boolean;
BEGIN
 uid:=nullif(current_setting('catalogdb.actor_id',true),'')::uuid;
 SELECT state='active' AND role IN('admin','editor') AND(NOT needs_admin OR role='admin') INTO ok FROM users WHERE id=uid FOR SHARE;
 IF NOT coalesce(ok,false) THEN RAISE EXCEPTION USING ERRCODE='CD012',MESSAGE='Active writer required';END IF;
 RETURN uid;
END $$;
CREATE FUNCTION catalog_choice(cat text,val text,previous text DEFAULT '') RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF coalesce(val,'')<>'' AND val IS DISTINCT FROM previous AND NOT EXISTS(SELECT 1 FROM lookup_values WHERE category=cat AND value=val) THEN
 RAISE EXCEPTION USING ERRCODE='CD011',MESSAGE='Dictionary value rejected',DETAIL=cat;
 END IF;
END $$;
CREATE FUNCTION catalog_prepare_row() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE k text;r jsonb;prev jsonb;v text;actor uuid;
BEGIN
 k:=CASE TG_TABLE_NAME WHEN 'servers' THEN 'server' WHEN 'instances' THEN 'instance' WHEN 'databases' THEN 'database' ELSE 'policy' END;
 actor:=nullif(current_setting('catalogdb.actor_id',true),'')::uuid;
 IF actor IS NOT NULL THEN NEW.updated_by:=catalog_actor(false); END IF;
 IF TG_OP='UPDATE' THEN NEW.version:=OLD.version+1;NEW.created_at:=OLD.created_at;prev:=to_jsonb(OLD);
 ELSE
  SELECT greatest(NEW.version,coalesce(max(version),0)+1) INTO NEW.version FROM record_versions WHERE entity=k AND object_id=NEW.id;
  NEW.version:=greatest(1,NEW.version);
 END IF;
 NEW.updated_at:=now();r:=to_jsonb(NEW);
 NEW.name:=btrim(NEW.name);
 IF k<>'policy' THEN
  IF EXISTS(SELECT 1 FROM jsonb_each(coalesce(r->'attributes','{}'))a(key,value)
   WHERE a.key IN('informationSystem','connectionMethod','domain','location','serviceOrganization','supervisor','responsible','osVersion','osFamily','personalData')
   AND(jsonb_typeof(a.value) NOT IN('string','null') OR length(a.value#>>'{}')>1000)) THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Invalid attribute type or length';END IF;
 END IF;
 IF octet_length(coalesce(r->>'description',''))>20000 THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Description too long';END IF;
 IF k='policy' THEN PERFORM catalog_choice('backupType',CASE WHEN NEW.backup_type='Не указано' THEN '' ELSE NEW.backup_type END);
 ELSE
  IF coalesce(NEW.status,'')='' THEN NEW.status:='Эксплуатируется';r:=to_jsonb(NEW);END IF;
  PERFORM catalog_choice('status',r->>'status',prev->>'status');
  PERFORM catalog_choice('environment',r->>'environment',prev->>'environment');
  IF k='instance' THEN
   IF coalesce(r->>'dbms','')='' THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='DBMS required'; END IF;
   PERFORM catalog_choice('dbms',r->>'dbms',prev->>'dbms');
  END IF;
  PERFORM catalog_choice('domain',r->'attributes'->>'domain',prev->'attributes'->>'domain');
  PERFORM catalog_choice('connectionMethod',r->'attributes'->>'connectionMethod',prev->'attributes'->>'connectionMethod');
  IF jsonb_typeof(r->'attributes'->'administration')='array' THEN
   IF jsonb_array_length(r->'attributes'->'administration')>30 THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Too many administration values';END IF;
   FOR v IN SELECT jsonb_array_elements_text(r->'attributes'->'administration') LOOP PERFORM catalog_choice('administration',v);END LOOP;
  ELSIF r->'attributes'->'administration' IS NOT NULL AND r->'attributes'->'administration'<>'null'::jsonb THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Administration must be an array';
  END IF;
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION catalog_object_path(k text,r jsonb) RETURNS text LANGUAGE plpgsql STABLE AS $$
DECLARE path text;parent uuid;
BEGIN
 IF k='server' THEN RETURN r->>'name';END IF;
 parent:=coalesce(r->>'instance_id',r->>'server_id')::uuid;
 IF k='instance' OR r->>'dbms'='Oracle Database' THEN
  SELECT name INTO path FROM servers WHERE id=parent;
  IF k='database' THEN path:=path||' / Oracle Database';END IF;
 ELSE
  SELECT s.name||' / '||i.name INTO path FROM instances i JOIN servers s ON s.id=i.server_id WHERE i.id=parent;
 END IF;
 RETURN path||' / '||(r->>'name');
END $$;
CREATE FUNCTION catalog_object_policy(k text,r jsonb) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT catalog_snapshot('policy',to_jsonb(p)) FROM backup_policies p
 WHERE(k='database' AND p.database_id=(r->>'id')::uuid)
 OR(p.instance_id=CASE WHEN k='instance' THEN(r->>'id')::uuid WHEN k='database' AND coalesce(r->>'dbms','')='' THEN(r->>'instance_id')::uuid END)
 ORDER BY(p.database_id IS NOT NULL) DESC LIMIT 1
$$;
CREATE FUNCTION catalog_capture_row() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE k text;act text;uid uuid;b jsonb;a jsonb;reason text;fp text;tp text;oldpolicy jsonb;newpolicy jsonb;batch uuid;
BEGIN
 k:=CASE TG_TABLE_NAME WHEN 'servers' THEN 'server' WHEN 'instances' THEN 'instance' WHEN 'databases' THEN 'database' ELSE 'policy' END;
 uid:=coalesce(nullif(current_setting('catalogdb.actor_id',true),'')::uuid,CASE WHEN TG_OP='DELETE' THEN OLD.updated_by ELSE NEW.updated_by END);
 IF TG_OP<>'INSERT' THEN b:=catalog_snapshot(k,to_jsonb(OLD)); END IF;
 IF TG_OP<>'DELETE' THEN a:=catalog_snapshot(k,to_jsonb(NEW)); END IF;
 act:=CASE TG_OP WHEN 'INSERT' THEN 'create' WHEN 'UPDATE' THEN 'update' ELSE 'delete' END;
 IF TG_OP<>'DELETE' AND current_setting('catalogdb.write_action',true)='restore' THEN act:='restore';END IF;
 IF TG_OP='DELETE' THEN
  IF NOT EXISTS(SELECT 1 FROM deleted_objects WHERE entity=k AND object_id=OLD.id AND restored_at IS NULL AND(snapshot->>'version')::bigint=OLD.version) THEN
   batch:=nullif(current_setting('catalogdb.delete_batch',true),'')::uuid;
   IF batch IS NULL THEN batch:=gen_random_uuid();PERFORM set_config('catalogdb.delete_batch',batch::text,true);END IF;
   INSERT INTO deleted_batches(id,kind,name,actor_id)VALUES(batch,k,OLD.name,uid) ON CONFLICT DO NOTHING;
   INSERT INTO deleted_objects(entity,object_id,batch_id,snapshot,deleted_by)VALUES(k,OLD.id,batch,b,uid)
   ON CONFLICT(entity,object_id)DO UPDATE SET batch_id=excluded.batch_id,snapshot=excluded.snapshot,deleted_by=excluded.deleted_by,deleted_at=now(),restored_at=NULL,restored_by=NULL;
  END IF;
 END IF;
 INSERT INTO object_history(entity,object_id,action,actor_id,before_state,after_state)VALUES(k,coalesce((a->>'id')::uuid,(b->>'id')::uuid),act,uid,b,a);
 INSERT INTO audit_events(actor_id,action,entity,object_id,details)VALUES(uid,act,k,coalesce((a->>'id')::uuid,(b->>'id')::uuid),jsonb_build_object('objectName',coalesce(a->>'name',b->>'name')));
 IF TG_OP='UPDATE' AND k IN('instance','database') AND (b->>'parentId' IS DISTINCT FROM a->>'parentId' OR (b->>'dbms'='Oracle Database') IS DISTINCT FROM(a->>'dbms'='Oracle Database')) THEN
  reason:=coalesce(nullif(current_setting('catalogdb.move_reason',true),''),CASE WHEN act='restore' THEN 'Возврат к сохранённой версии' ELSE 'Изменение родителя в карточке' END);
  fp:=catalog_object_path(k,to_jsonb(OLD));tp:=catalog_object_path(k,to_jsonb(NEW));
  oldpolicy:=catalog_object_policy(k,to_jsonb(OLD));newpolicy:=catalog_object_policy(k,to_jsonb(NEW));
  INSERT INTO object_movements(entity,object_id,from_parent,to_parent,from_path,to_path,reason,from_policy,to_policy,object_version,actor_id)
   VALUES(k,NEW.id,(b->>'parentId')::uuid,(a->>'parentId')::uuid,fp,tp,reason,oldpolicy,newpolicy,NEW.version,uid);
  IF k='instance' THEN
   INSERT INTO object_movements(entity,object_id,from_parent,to_parent,from_path,to_path,reason,from_policy,to_policy,object_version,actor_id)
   SELECT 'database',d.id,NEW.id,NEW.id,fp||' / '||d.name,tp||' / '||d.name,'Перенос родительского экземпляра: '||reason,
   catalog_object_policy('database',to_jsonb(d)),catalog_object_policy('database',to_jsonb(d)),d.version,uid FROM databases d WHERE d.instance_id=NEW.id;
  END IF;
  INSERT INTO audit_events(actor_id,action,entity,object_id,details)VALUES(uid,'object_move',k,NEW.id,jsonb_build_object('from',fp,'to',tp,'reason',reason,'version',NEW.version));
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;END IF;
 RETURN NEW;
END $$;
DO $$ DECLARE tbl text; BEGIN
 FOREACH tbl IN ARRAY ARRAY['servers','instances','databases','backup_policies'] LOOP
  EXECUTE format('CREATE TRIGGER catalog_prepare BEFORE INSERT OR UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION catalog_prepare_row()',tbl);
  EXECUTE format('CREATE TRIGGER catalog_capture AFTER INSERT OR UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION catalog_capture_row()',tbl);
  EXECUTE format('CREATE TRIGGER catalog_delete BEFORE DELETE ON %I FOR EACH ROW EXECUTE FUNCTION catalog_capture_row()',tbl);
 END LOOP;
END $$;

-- Bound names and columns come exclusively from pg_catalog and the four allowed kinds.
CREATE FUNCTION api_save_record(k text,p jsonb,expected bigint,mode integer DEFAULT 0,reason text DEFAULT '') RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE tbl text;uid uuid;oid uuid;oldrow jsonb;r jsonb;result jsonb;cols text;vals text;creating boolean;parent uuid;
BEGIN
 IF k NOT IN('server','instance','database','policy') OR mode NOT IN(0,1,2) THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Invalid write operation';END IF;
 uid:=catalog_actor(mode<>0);
 tbl:=CASE k WHEN 'server' THEN 'servers' WHEN 'instance' THEN 'instances' WHEN 'database' THEN 'databases' ELSE 'backup_policies' END;
 oid:=nullif(p->>'id','')::uuid;creating:=oid IS NULL OR mode=2 OR expected=0;
 IF oid IS NULL THEN oid:=gen_random_uuid();END IF;
 IF creating AND mode<>2 AND(EXISTS(SELECT 1 FROM record_versions WHERE entity=k AND object_id=oid) OR EXISTS(SELECT 1 FROM deleted_objects WHERE entity=k AND object_id=oid)) THEN RAISE EXCEPTION USING ERRCODE='CD012',MESSAGE='Recreation requires administrator restore';END IF;
 IF NOT creating THEN
  EXECUTE format('SELECT to_jsonb(t) FROM %I t WHERE id=$1 FOR UPDATE',tbl) INTO oldrow USING oid;
  IF oldrow IS NULL THEN RAISE EXCEPTION USING ERRCODE='CD013',MESSAGE='Object not found';END IF;
  IF(oldrow->>'version')::bigint<>expected THEN RAISE EXCEPTION USING ERRCODE='CD014',MESSAGE='Object changed';END IF;
 END IF;
 PERFORM set_config('catalogdb.write_action',CASE WHEN mode<>0 THEN 'restore' ELSE '' END,true);
 PERFORM set_config('catalogdb.move_reason',coalesce(reason,''),true);
 r:=coalesce(oldrow,'{}')||p||jsonb_build_object('id',oid,'updated_by',uid,'created_at',coalesce(oldrow->'created_at',to_jsonb(now())),'updated_at',now(),'version',greatest(1,expected+1));
 IF k='policy' THEN
  parent:=nullif(p->>'objectId','')::uuid;
  IF p->>'kind' NOT IN('instance','database') OR parent IS NULL THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Invalid policy target';END IF;
  r:=r||jsonb_build_object('instance_id',CASE WHEN p->>'kind'='instance' THEN parent END,'database_id',CASE WHEN p->>'kind'='database' THEN parent END,
  'backup_type',p->>'backupType','retention_days',p->'retentionDays');
 ELSE
  parent:=nullif(p->>'parentId','')::uuid;
  IF k<>'server' AND parent IS NULL THEN RAISE EXCEPTION USING ERRCODE='CD010',MESSAGE='Parent required';END IF;
  r:=r||jsonb_build_object('server_id',CASE WHEN k='instance' OR(k='database' AND p->>'dbms'='Oracle Database') THEN parent END,
   'instance_id',CASE WHEN k='database' AND coalesce(p->>'dbms','')='' THEN parent END,'dbms_version',coalesce(p->>'dbmsVersion',''));
 END IF;
 SELECT string_agg(format('%I',a.attname),',' ORDER BY a.attnum),string_agg(format('x.%I',a.attname),',' ORDER BY a.attnum) INTO cols,vals
 FROM pg_attribute a WHERE a.attrelid=format('%I.%I',current_schema(),tbl)::regclass AND a.attnum>0 AND NOT a.attisdropped AND a.attgenerated='' AND a.attidentity='';
 IF creating THEN
  EXECUTE format('INSERT INTO %I AS t(%s) SELECT %s FROM jsonb_populate_record(NULL::%I,$1)x RETURNING to_jsonb(t)',tbl,cols,vals,tbl) INTO result USING r;
 ELSE
  EXECUTE format('UPDATE %I AS t SET(%s)=(SELECT %s FROM jsonb_populate_record(NULL::%I,$1)x) WHERE id=$2 RETURNING to_jsonb(t)',tbl,cols,vals,tbl) INTO result USING r,oid;
 END IF;
 PERFORM set_config('catalogdb.write_action','',true);PERFORM set_config('catalogdb.move_reason','',true);
 RETURN catalog_snapshot(k,result);
END $$;
CREATE FUNCTION api_delete_records(sids uuid[],iids uuid[],dids uuid[],pids uuid[],batch uuid,root_kind text,root_name text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE uid uuid;
BEGIN
 uid:=catalog_actor(true);
 INSERT INTO deleted_batches(id,kind,name,actor_id)VALUES(batch,root_kind,root_name,uid);
 INSERT INTO deleted_objects(entity,object_id,batch_id,snapshot,deleted_by)
 SELECT k,id,batch,catalog_snapshot(k,r),uid FROM(
 SELECT 'server' k,id,to_jsonb(s)r FROM servers s WHERE id=ANY(sids)
 UNION ALL SELECT 'instance',id,to_jsonb(i) FROM instances i WHERE id=ANY(iids)
 UNION ALL SELECT 'database',id,to_jsonb(d) FROM databases d WHERE id=ANY(dids)
 UNION ALL SELECT 'policy',id,to_jsonb(p) FROM backup_policies p WHERE id=ANY(pids)
 )x ON CONFLICT(entity,object_id)DO UPDATE SET batch_id=excluded.batch_id,snapshot=excluded.snapshot,deleted_by=excluded.deleted_by,deleted_at=now(),restored_at=NULL,restored_by=NULL;
 DELETE FROM backup_policies WHERE id=ANY(pids);
 DELETE FROM databases WHERE id=ANY(dids);
 DELETE FROM instances WHERE id=ANY(iids);
 DELETE FROM servers WHERE id=ANY(sids);
END $$;
CREATE FUNCTION api_restore_records(refs jsonb) RETURNS integer LANGUAGE plpgsql AS $$
DECLARE r record;n int:=0;expected bigint;
BEGIN
 PERFORM catalog_actor(true);
 FOR r IN SELECT d.* FROM deleted_objects d JOIN jsonb_to_recordset(refs)x(kind text,id uuid) ON x.kind=d.entity AND x.id=d.object_id
 WHERE d.restored_at IS NULL ORDER BY CASE d.entity WHEN 'server' THEN 0 WHEN 'instance' THEN 1 WHEN 'database' THEN 2 ELSE 3 END,d.object_id
 LOOP
  IF r.entity='server' AND EXISTS(SELECT 1 FROM servers WHERE lower(btrim(name))=lower(btrim(r.snapshot->>'name'))) THEN RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='Server name conflict';END IF;
  SELECT coalesce(max(version),0) INTO expected FROM record_versions WHERE entity=r.entity AND object_id=r.object_id;
  PERFORM api_save_record(r.entity,r.snapshot,expected,2);
  UPDATE deleted_objects SET restored_at=now(),restored_by=catalog_actor(true) WHERE entity=r.entity AND object_id=r.object_id;
  n:=n+1;
 END LOOP;
 IF n<>jsonb_array_length(refs) THEN RAISE EXCEPTION USING ERRCODE='CD014',MESSAGE='Restore selection changed';END IF;
 RETURN n;
END $$;
DO $$ DECLARE f record; BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('catalog_insert_history','catalog_actor','catalog_choice','catalog_prepare_row','catalog_object_path','catalog_object_policy','catalog_capture_row','api_save_record','api_delete_records','api_restore_records')
 LOOP EXECUTE format('ALTER FUNCTION %s SET search_path TO %I,pg_catalog,pg_temp',f.signature,current_schema()); END LOOP;
END $$;

$catalog_source$;
INSERT INTO schema_migrations(version,name,checksum)VALUES(12,'012_server_writes_and_history.sql','c5741281ed6fdac9239692d250264bb1eb17ccced0157234b5830e456a6f91ad');
END IF; END $catalog_step$;
-- 013_notifications_and_live_events.sql
DO $catalog_step$ BEGIN
IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=13) THEN
EXECUTE $catalog_source$
CREATE FUNCTION catalog_audit_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE title text;objname text;
BEGIN
 CASE NEW.action
 WHEN 'register' THEN
  INSERT INTO notifications(user_id,category,title,body,entity,object_id)
   SELECT a.id,'registration','Заявка на регистрацию',u.login||' ожидает подтверждения','user',u.id
   FROM users a CROSS JOIN users u WHERE a.role='admin' AND a.state='active' AND u.id=NEW.object_id AND u.state='pending';
 WHEN 'user_password_change' THEN
  INSERT INTO notifications(user_id,category,title,body,entity,object_id)
   SELECT id,'account','Пароль изменён','Администратор изменил пароль вашей учётной записи. Новый пароль применяется при следующем входе.','account',id FROM users WHERE id=NEW.object_id;
 WHEN 'user_update' THEN
  INSERT INTO notifications(user_id,category,title,body,entity,object_id)
   SELECT id,'account','Изменена учётная запись','Роль: '||CASE role WHEN 'admin' THEN 'Администратор' WHEN 'editor' THEN 'Редактор' ELSE 'Наблюдатель' END||
   '. Состояние: '||CASE state WHEN 'active' THEN 'Активна' WHEN 'pending' THEN 'Ожидает подтверждения' WHEN 'blocked' THEN 'Заблокирована' ELSE 'Отключена' END,'account',id FROM users WHERE id=NEW.object_id;
 ELSE
  IF NEW.action IN('update','delete','restore') AND NEW.entity IN('server','instance','database') THEN
   title:=CASE WHEN NEW.action='delete' THEN 'Удалён избранный объект' ELSE 'Изменён избранный объект' END;
   objname:=coalesce(NEW.details->>'objectName',(SELECT coalesce(after_state->>'name',before_state->>'name') FROM object_history WHERE entity=NEW.entity AND object_id=NEW.object_id ORDER BY id DESC LIMIT 1),'Объект каталога');
   INSERT INTO notifications(user_id,category,title,body,entity,object_id)
    SELECT f.user_id,'favorite',title,objname,NEW.entity,NEW.object_id FROM favorites f JOIN users u ON u.id=f.user_id
    WHERE(CASE NEW.entity WHEN 'server' THEN f.server_id WHEN 'instance' THEN f.instance_id ELSE f.database_id END)=NEW.object_id
    AND f.user_id IS DISTINCT FROM NEW.actor_id AND u.state='active';
  END IF;
 END CASE;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_notifications AFTER INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION catalog_audit_notifications();
CREATE FUNCTION catalog_publish_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- Same payload is deliberately coalesced by PostgreSQL within a transaction.
 PERFORM pg_notify('catalogdb_events',json_build_object('schema',TG_TABLE_SCHEMA,'type',TG_ARGV[0])::text);
 RETURN NULL;
END $$;
DO $$ DECLARE tbl text;BEGIN
 FOREACH tbl IN ARRAY ARRAY['servers','instances','databases','backup_policies','object_attachments','lookup_values'] LOOP
  EXECUTE format('CREATE TRIGGER catalog_live AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH STATEMENT EXECUTE FUNCTION catalog_publish_change(''catalog_changed'')',tbl);
 END LOOP;
END $$;
CREATE TRIGGER notifications_live AFTER INSERT OR UPDATE OR DELETE ON notifications FOR EACH STATEMENT EXECUTE FUNCTION catalog_publish_change('notifications_changed');
CREATE TRIGGER users_live AFTER UPDATE ON users FOR EACH STATEMENT EXECUTE FUNCTION catalog_publish_change('users_changed');
DO $$ DECLARE f record;BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('catalog_audit_notifications')
 LOOP EXECUTE format('ALTER FUNCTION %s SET search_path TO %I,pg_catalog,pg_temp',f.signature,current_schema());END LOOP;
END $$;

$catalog_source$;
INSERT INTO schema_migrations(version,name,checksum)VALUES(13,'013_notifications_and_live_events.sql','eabc713b9003caca20ac3430d0c88ba459a8438f6e3cd9341efeb04408e14356');
END IF; END $catalog_step$;
-- 014_saved_sessions.sql
DO $catalog_step$ BEGIN
IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=14) THEN
EXECUTE $catalog_source$
CREATE TABLE user_sessions(
 id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 token_hash text NOT NULL CHECK(length(token_hash)=64),
 created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL,
 revoked_at timestamptz,CHECK(expires_at>created_at AND expires_at<=created_at+interval '168 hours')
);
CREATE INDEX user_sessions_owner ON user_sessions(user_id) WHERE revoked_at IS NULL;
CREATE INDEX user_sessions_expiry ON user_sessions(expires_at);
CREATE FUNCTION catalog_revoke_sessions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.password_hash IS DISTINCT FROM OLD.password_hash OR(NEW.state IS DISTINCT FROM OLD.state AND NEW.state<>'active') THEN
 UPDATE user_sessions SET revoked_at=now() WHERE user_id=NEW.id AND revoked_at IS NULL;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER users_revoke_sessions AFTER UPDATE ON users FOR EACH ROW EXECUTE FUNCTION catalog_revoke_sessions();
DO $$ BEGIN EXECUTE format('ALTER FUNCTION catalog_revoke_sessions() SET search_path TO %I,pg_catalog,pg_temp',current_schema());END $$;

$catalog_source$;
INSERT INTO schema_migrations(version,name,checksum)VALUES(14,'014_saved_sessions.sql','772727396e6421a82c68353cfb4392b3f00eae0d59b3f52af2937f1e3111491a');
END IF; END $catalog_step$;
-- 015_batch_and_reporting_api.sql
DO $catalog_step$ BEGIN
IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=15) THEN
EXECUTE $catalog_source$
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

$catalog_source$;
INSERT INTO schema_migrations(version,name,checksum)VALUES(15,'015_batch_and_reporting_api.sql','d863d1537ca500adfb5352cff85fdde2ce99e7d6612d9d7de546524f59a45433');
END IF; END $catalog_step$;
-- 016_move_api_and_invariants.sql
DO $catalog_step$ BEGIN
IF NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=16) THEN
EXECUTE $catalog_source$
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

$catalog_source$;
INSERT INTO schema_migrations(version,name,checksum)VALUES(16,'016_move_api_and_invariants.sql','813ab061a35d72b5dfadca1fb3a9478717301cc1e7732aaa73fc124af65407d8');
END IF; END $catalog_step$;
COMMIT;
-- Upgrade complete. Re-running this file checks the ledger and skips applied steps.
