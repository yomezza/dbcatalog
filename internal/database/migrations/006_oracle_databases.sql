-- Oracle SID is a database directly owned by its server in the catalog model.
ALTER TABLE databases ADD COLUMN server_id uuid REFERENCES servers(id) ON DELETE RESTRICT;
ALTER TABLE databases ADD COLUMN dbms text NOT NULL DEFAULT '';
ALTER TABLE databases ADD COLUMN dbms_version text NOT NULL DEFAULT '';
ALTER TABLE databases ADD COLUMN port integer CHECK(port BETWEEN 1 AND 65535);
ALTER TABLE databases ALTER COLUMN instance_id DROP NOT NULL;

-- Refuse ambiguous conversion instead of merging or dropping independent records.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM databases d JOIN instances i ON i.id=d.instance_id
           WHERE i.dbms='Oracle Database' GROUP BY i.server_id,d.name HAVING count(*)>1) THEN
  RAISE EXCEPTION USING ERRCODE='CD001',MESSAGE='Conflicting Oracle SIDs within a server';
 END IF;
END $$;

CREATE TEMP TABLE catalogdb_oracle_instances ON COMMIT DROP AS
 SELECT i.* FROM instances i WHERE i.dbms='Oracle Database';
CREATE TEMP TABLE catalogdb_oracle_databases ON COMMIT DROP AS
 SELECT d.*,i.server_id AS target_server_id,i.dbms_version AS target_dbms_version,i.port AS target_port
 FROM databases d JOIN catalogdb_oracle_instances i ON i.id=d.instance_id;
CREATE TEMP TABLE catalogdb_oracle_policies ON COMMIT DROP AS
 SELECT p.* FROM backup_policies p JOIN catalogdb_oracle_instances i ON i.id=p.instance_id;

-- Preserve each legacy instance, policy and relationship in immutable history before conversion.
INSERT INTO object_history(entity,object_id,action,actor_id,before_state,after_state)
 SELECT 'instance',i.id,'delete',i.updated_by,
  to_jsonb(i)||jsonb_build_object('kind','instance','parentId',i.server_id,'dbmsVersion',i.dbms_version),NULL
 FROM catalogdb_oracle_instances i;
INSERT INTO object_history(entity,object_id,action,actor_id,before_state,after_state)
 SELECT 'policy',p.id,'delete',p.updated_by,
  to_jsonb(p)||jsonb_build_object('kind','instance','objectId',p.instance_id,'backupType',p.backup_type,'retentionDays',p.retention_days),NULL
 FROM catalogdb_oracle_policies p;

UPDATE databases d SET server_id=o.target_server_id,instance_id=NULL,dbms='Oracle Database',
 dbms_version=o.target_dbms_version,port=o.target_port,version=d.version+1,updated_at=now()
 FROM catalogdb_oracle_databases o WHERE o.id=d.id;
INSERT INTO object_history(entity,object_id,action,actor_id,before_state,after_state)
 SELECT 'database',d.id,'update',d.updated_by,
  (to_jsonb(o)-'target_server_id'-'target_dbms_version'-'target_port')||jsonb_build_object('kind','database','parentId',o.instance_id,'dbmsVersion',''),
  to_jsonb(d)||jsonb_build_object('kind','database','parentId',d.server_id,'dbmsVersion',d.dbms_version)
 FROM databases d JOIN catalogdb_oracle_databases o ON o.id=d.id;

-- Materialize the former effective instance policy only where no explicit DB policy exists.
INSERT INTO backup_policies(id,database_id,name,enabled,backup_type,scheduler,schedule,storage,retention_days,description,updated_by)
 SELECT md5(p.id::text||':'||d.id::text)::uuid,d.id,p.name,p.enabled,p.backup_type,p.scheduler,p.schedule,p.storage,p.retention_days,p.description,p.updated_by
 FROM catalogdb_oracle_policies p JOIN catalogdb_oracle_databases d ON d.instance_id=p.instance_id
 WHERE NOT EXISTS(SELECT 1 FROM backup_policies own WHERE own.database_id=d.id);
INSERT INTO object_history(entity,object_id,action,actor_id,before_state,after_state)
 SELECT 'policy',p.id,'create',p.updated_by,NULL,
  to_jsonb(p)||jsonb_build_object('kind','database','objectId',p.database_id,'backupType',p.backup_type,'retentionDays',p.retention_days)
 FROM backup_policies p JOIN catalogdb_oracle_databases d ON d.id=p.database_id
 JOIN catalogdb_oracle_policies old ON p.id=md5(old.id::text||':'||d.id::text)::uuid;

INSERT INTO favorites(user_id,database_id)
 SELECT f.user_id,d.id FROM favorites f JOIN catalogdb_oracle_databases d ON d.instance_id=f.instance_id
 ON CONFLICT(user_id,database_id) DO NOTHING;
DELETE FROM backup_policies WHERE instance_id IN(SELECT id FROM catalogdb_oracle_instances);
DELETE FROM instances WHERE id IN(SELECT id FROM catalogdb_oracle_instances);

-- Persist original snapshots and the reason for structural changes in the audit as well.
INSERT INTO audit_events(actor_id,action,entity,object_id,details)
 SELECT updated_by,'oracle_migration','instance',id,jsonb_build_object('reason','Oracle: базы перенесены непосредственно под сервер; исходная запись сохранена в истории') FROM catalogdb_oracle_instances;
INSERT INTO audit_events(actor_id,action,entity,object_id,details)
 SELECT updated_by,'oracle_migration','database',id,jsonb_build_object('reason','Oracle: родитель изменён с экземпляра на сервер') FROM catalogdb_oracle_databases;
INSERT INTO notifications(user_id,category,title,body,entity)
 SELECT id,'administration','Обновлена структура Oracle Database','Базы Oracle перенесены непосредственно под серверы. Политики и избранное перенесены, исходные связи сохранены в истории.','catalog'
 FROM users WHERE role='admin' AND state='active' AND EXISTS(SELECT 1 FROM catalogdb_oracle_instances);

ALTER TABLE databases ADD CONSTRAINT databases_parent_model CHECK(
 (instance_id IS NOT NULL AND server_id IS NULL AND dbms='' AND dbms_version='' AND port IS NULL) OR
 (instance_id IS NULL AND server_id IS NOT NULL AND dbms='Oracle Database')
);
CREATE UNIQUE INDEX databases_oracle_sid ON databases(server_id,name) WHERE server_id IS NOT NULL;
CREATE INDEX databases_server ON databases(server_id) WHERE server_id IS NOT NULL;
-- New Oracle instances are no longer part of this model.
ALTER TABLE instances ADD CONSTRAINT instances_not_oracle CHECK(dbms<>'Oracle Database');
