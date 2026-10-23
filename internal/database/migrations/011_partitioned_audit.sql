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
