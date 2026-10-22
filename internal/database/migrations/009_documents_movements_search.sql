CREATE TABLE object_attachments(
 id uuid PRIMARY KEY,entity text NOT NULL CHECK(entity IN('server','instance','database')),object_id uuid NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),description text NOT NULL DEFAULT '',
 file_name text NOT NULL,mime_type text NOT NULL,content bytea NOT NULL CHECK(octet_length(content) BETWEEN 1 AND 20971520),
 sha256 text NOT NULL,document jsonb,version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_by uuid NOT NULL REFERENCES users(id),updated_by uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
-- Keep attachment identity through object deletion and restoration. Access requires a live parent.
CREATE INDEX object_attachments_parent ON object_attachments(entity,object_id,created_at,id);
CREATE TABLE object_movements(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,entity text NOT NULL CHECK(entity IN('instance','database')),
 object_id uuid NOT NULL,from_parent uuid NOT NULL,to_parent uuid NOT NULL,
 from_path text NOT NULL,to_path text NOT NULL,reason text NOT NULL,
 from_policy jsonb,to_policy jsonb,object_version bigint NOT NULL,actor_id uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX object_movements_object ON object_movements(entity,object_id,id DESC);
CREATE FUNCTION catalog_search_text(
 n text,a text,o text,e text,s text,d text,m text,v text,p integer,u text,attrs jsonb
) RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
 SELECT lower(concat_ws(' ',n,a,o,e,s,d,m,v,p::text,u,
 attrs->>'connectionMethod',attrs->>'domain',attrs->>'location',attrs->>'serviceOrganization',
 attrs->>'supervisor',attrs->>'responsible',attrs->>'osVersion',attrs->>'osFamily',
 attrs->>'personalData',attrs->>'informationSystem',
 (SELECT string_agg(value,' ') FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(attrs->'administration')='array' THEN attrs->'administration' ELSE '[]' END) t(value))))
$$;
ALTER TABLE servers ADD COLUMN search_text text GENERATED ALWAYS AS
 (catalog_search_text(name,address,os,environment,status,description,'','',NULL,'',attributes)) STORED;
ALTER TABLE instances ADD COLUMN search_text text GENERATED ALWAYS AS
 (catalog_search_text(name,'','',environment,status,description,dbms,dbms_version,port,'',attributes)) STORED;
ALTER TABLE databases ADD COLUMN search_text text GENERATED ALWAYS AS
 (catalog_search_text(name,'','',environment,status,description,dbms,dbms_version,port,purpose,attributes)) STORED;
CREATE INDEX servers_catalog_order ON servers(lower(name),id);
CREATE INDEX instances_catalog_order ON instances(server_id,lower(name),id);
CREATE INDEX databases_catalog_instance_order ON databases(instance_id,lower(name),id);
CREATE INDEX databases_catalog_server_order ON databases(server_id,lower(name),id) WHERE server_id IS NOT NULL;

CREATE FUNCTION catalog_document_text(n text,d text,f text,doc jsonb)
 RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
 SELECT lower(concat_ws(' ',n,d,f,doc->>'title',doc->>'purpose',doc->>'prerequisites',doc->>'steps',doc->>'verification',doc->>'rollback',doc->>'notes'))
$$;
ALTER TABLE object_attachments ADD COLUMN search_text text GENERATED ALWAYS AS
 (catalog_document_text(name,description,file_name,document)) STORED;
