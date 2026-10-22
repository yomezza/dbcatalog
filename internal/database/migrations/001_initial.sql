CREATE TABLE users (
 id uuid PRIMARY KEY,
 login text NOT NULL CHECK (length(login) BETWEEN 3 AND 80),
 password_hash text NOT NULL,
 role text NOT NULL CHECK (role IN ('admin','editor','viewer')),
 state text NOT NULL CHECK (state IN ('pending','active','blocked','disabled')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_login_key ON users(lower(login));
CREATE TABLE servers (
 id uuid PRIMARY KEY,
 name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 address text NOT NULL DEFAULT '',
 os text NOT NULL DEFAULT '',
 environment text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'Активен',
 description text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 updated_by uuid NOT NULL REFERENCES users(id)
);
CREATE TABLE instances (
 id uuid PRIMARY KEY,
 server_id uuid NOT NULL REFERENCES servers(id) ON DELETE RESTRICT,
 name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 dbms text NOT NULL,
 dbms_version text NOT NULL DEFAULT '',
 port integer CHECK(port BETWEEN 1 AND 65535),
 status text NOT NULL DEFAULT 'Активен',
 description text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 updated_by uuid NOT NULL REFERENCES users(id),
 UNIQUE(server_id,name)
);
CREATE TABLE databases (
 id uuid PRIMARY KEY,
 instance_id uuid NOT NULL REFERENCES instances(id) ON DELETE RESTRICT,
 name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 purpose text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'Активна',
 description text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 updated_by uuid NOT NULL REFERENCES users(id),
 UNIQUE(instance_id,name)
);
CREATE TABLE object_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 entity text NOT NULL,
 object_id uuid NOT NULL,
 action text NOT NULL,
 actor_id uuid NOT NULL REFERENCES users(id),
 before_state jsonb,
 after_state jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX object_history_lookup ON object_history(entity,object_id,created_at DESC);
CREATE TABLE audit_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor_id uuid REFERENCES users(id),
 action text NOT NULL,
 entity text NOT NULL DEFAULT '',
 object_id uuid,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_created ON audit_events(created_at DESC);
CREATE INDEX instances_parent ON instances(server_id);
CREATE INDEX databases_parent ON databases(instance_id);
