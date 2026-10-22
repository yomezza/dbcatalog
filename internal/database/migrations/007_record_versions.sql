-- Version snapshots survive deletion and are separate from the paginated change journal.
LOCK TABLE servers,instances,databases,backup_policies,object_history IN SHARE ROW EXCLUSIVE MODE;
CREATE INDEX object_history_sequence ON object_history(entity,object_id,id DESC);
CREATE TABLE record_versions (
 entity text NOT NULL CHECK(entity IN ('server','instance','database','policy')),
 object_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 actor_id uuid NOT NULL REFERENCES users(id),
 action text NOT NULL,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(entity,object_id,version)
);
-- Prefer a recorded after-state over a later before-state of the same version.
INSERT INTO record_versions(entity,object_id,version,snapshot,actor_id,action,created_at)
 SELECT DISTINCT ON(entity,object_id,v) entity,object_id,v,snapshot,actor_id,action,created_at
 FROM (
  SELECT h.entity,h.object_id,(s.snapshot->>'version')::bigint AS v,s.snapshot,h.actor_id,h.action,h.created_at,h.id,s.priority
  FROM object_history h CROSS JOIN LATERAL (VALUES(h.after_state,1),(h.before_state,0)) s(snapshot,priority)
  WHERE h.entity IN ('server','instance','database','policy') AND jsonb_typeof(s.snapshot)='object'
   AND s.snapshot->>'version' ~ '^[1-9][0-9]*$'
 ) candidates ORDER BY entity,object_id,v,priority DESC,id DESC;

-- Seed a current snapshot even for records originating before a history mechanism existed.
INSERT INTO record_versions(entity,object_id,version,snapshot,actor_id,action,created_at)
 SELECT 'server',id,version,to_jsonb(s)||jsonb_build_object('kind','server','parentId',''),updated_by,'baseline',updated_at FROM servers s
 ON CONFLICT DO NOTHING;
INSERT INTO record_versions(entity,object_id,version,snapshot,actor_id,action,created_at)
 SELECT 'instance',id,version,to_jsonb(i)||jsonb_build_object('kind','instance','parentId',server_id,'dbmsVersion',dbms_version),updated_by,'baseline',updated_at FROM instances i
 ON CONFLICT DO NOTHING;
INSERT INTO record_versions(entity,object_id,version,snapshot,actor_id,action,created_at)
 SELECT 'database',id,version,to_jsonb(d)||jsonb_build_object('kind','database','parentId',coalesce(server_id,instance_id),'dbmsVersion',dbms_version),updated_by,'baseline',updated_at FROM databases d
 ON CONFLICT DO NOTHING;
INSERT INTO record_versions(entity,object_id,version,snapshot,actor_id,action,created_at)
 SELECT 'policy',id,version,to_jsonb(p)||jsonb_build_object('kind',CASE WHEN database_id IS NULL THEN 'instance' ELSE 'database' END,'objectId',coalesce(database_id,instance_id),'backupType',backup_type,'retentionDays',retention_days),updated_by,'baseline',updated_at FROM backup_policies p
 ON CONFLICT DO NOTHING;

CREATE FUNCTION capture_record_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.entity IN ('server','instance','database','policy') AND jsonb_typeof(NEW.after_state)='object'
    AND NEW.after_state->>'version' ~ '^[1-9][0-9]*$' THEN
  EXECUTE pg_catalog.format('INSERT INTO %I.record_versions(entity,object_id,version,snapshot,actor_id,action,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)',TG_TABLE_SCHEMA)
   USING NEW.entity,NEW.object_id,(NEW.after_state->>'version')::bigint,NEW.after_state,NEW.actor_id,NEW.action,NEW.created_at;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER object_history_version AFTER INSERT ON object_history FOR EACH ROW EXECUTE FUNCTION capture_record_version();
