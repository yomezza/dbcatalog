CREATE TABLE saved_filters (
 id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES users(id),name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 100),
 criteria jsonb NOT NULL CHECK(jsonb_typeof(criteria)='object'),version bigint NOT NULL DEFAULT 1,updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX saved_filters_owner_name ON saved_filters(user_id,lower(trim(name)));
CREATE TABLE deleted_batches(id uuid PRIMARY KEY,kind text NOT NULL,name text NOT NULL,actor_id uuid NOT NULL REFERENCES users(id),deleted_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE deleted_objects(
 entity text NOT NULL CHECK(entity IN('server','instance','database','policy')),object_id uuid NOT NULL,
 batch_id uuid NOT NULL REFERENCES deleted_batches(id),snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 deleted_by uuid NOT NULL REFERENCES users(id),deleted_at timestamptz NOT NULL DEFAULT now(),restored_at timestamptz,restored_by uuid REFERENCES users(id),
 PRIMARY KEY(entity,object_id)
);
CREATE INDEX deleted_objects_active ON deleted_objects(deleted_at DESC,entity,object_id) WHERE restored_at IS NULL;
CREATE INDEX deleted_objects_batch ON deleted_objects(batch_id) WHERE restored_at IS NULL;
-- Recover deletion snapshots from prior releases, without resurrecting retired Oracle scaffolding.
WITH latest AS(
 SELECT DISTINCT ON(h.entity,h.object_id) h.* FROM object_history h
 WHERE action='delete' AND entity IN('server','instance','database','policy') AND jsonb_typeof(before_state)='object'
 ORDER BY entity,object_id,id DESC
),candidates AS(
 SELECT h.*,md5(h.actor_id::text||':'||h.created_at::text)::uuid AS batch FROM latest h
 WHERE NOT EXISTS(SELECT 1 FROM servers WHERE h.entity='server' AND id=h.object_id)
 AND NOT EXISTS(SELECT 1 FROM instances WHERE h.entity='instance' AND id=h.object_id)
 AND NOT EXISTS(SELECT 1 FROM databases WHERE h.entity='database' AND id=h.object_id)
 AND NOT EXISTS(SELECT 1 FROM backup_policies WHERE h.entity='policy' AND id=h.object_id)
 AND NOT(h.entity='instance' AND h.before_state->>'dbms'='Oracle Database')
 AND NOT(h.entity='policy' AND EXISTS(SELECT 1 FROM object_history old WHERE old.entity='instance' AND old.object_id::text=h.before_state->>'objectId' AND old.before_state->>'dbms'='Oracle Database'))
),batches AS(
 INSERT INTO deleted_batches(id,kind,name,actor_id,deleted_at)
 SELECT DISTINCT batch,'legacy','Архив удаления',actor_id,created_at FROM candidates ON CONFLICT DO NOTHING RETURNING id
)
INSERT INTO deleted_objects(entity,object_id,batch_id,snapshot,deleted_by,deleted_at)
 SELECT entity,object_id,batch,before_state,actor_id,created_at FROM candidates;
