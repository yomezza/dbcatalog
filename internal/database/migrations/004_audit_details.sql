ALTER TABLE audit_events ADD COLUMN details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(details)='object');
CREATE INDEX audit_events_actor_created ON audit_events(actor_id,created_at DESC,id DESC);
CREATE INDEX audit_events_object ON audit_events(entity,object_id,id DESC);
