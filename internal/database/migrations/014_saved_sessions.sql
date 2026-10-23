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
