package database

import (
	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/security"
	"testing"
	"testing/fstest"
	"time"
)

func TestBackendUpgradePreserves009DataAndMaintainsPartitions(t *testing.T) {
	ctx, p := testPool(t)
	schema := testSchema(t, ctx, p)
	prior := fstest.MapFS{}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() >= "010_" {
			continue
		}
		name := "migrations/" + e.Name()
		raw, e := migrationFiles.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		prior[name] = &fstest.MapFile{Data: raw}
	}
	if err = migrateFS(ctx, p, schema, prior); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	actor, server, instance, db, oracle := security.ID(), security.ID(), security.ID(), security.ID(), security.ID()
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,login,password_hash,role,state)VALUES($1,'legacy-admin','synthetic','admin','active')`, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO servers(id,name,address,updated_by)VALUES($1,'legacy-host','10.0.0.1',$2)`, server, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO instances(id,server_id,name,dbms,updated_by)VALUES($1,$2,'pg','PostgreSQL',$3)`, instance, server, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO databases(id,instance_id,name,updated_by)VALUES($1,$2,'ordinary',$3); `, db, instance, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO databases(id,server_id,name,dbms,updated_by)VALUES($1,$2,'ORA','Oracle Database',$3)`, oracle, server, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO backup_policies(id,instance_id,name,updated_by)VALUES(gen_random_uuid(),$1,'inherited',$2)`, instance, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO object_history(entity,object_id,action,actor_id,after_state) SELECT 'server',id,'create',$1,to_jsonb(s)||jsonb_build_object('kind','server','parentId','') FROM servers s;
 INSERT INTO object_history(entity,object_id,action,actor_id,before_state,after_state) SELECT 'server',id,'update',$1,to_jsonb(s)||jsonb_build_object('kind','server','parentId','','legacyExtra','preserve'),to_jsonb(s)||jsonb_build_object('kind','server','parentId','','version',2)FROM servers s;
 UPDATE servers SET version=2;
 INSERT INTO audit_events(actor_id,action,entity,object_id,details,created_at) SELECT $1,'legacy-sample','server',$2,jsonb_build_object('n',g),'2023-01-15'::timestamptz+g*interval '1 month' FROM generate_series(0,35)g;`, pgx.QueryExecModeSimpleProtocol, actor, server); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO object_attachments(id,entity,object_id,name,file_name,mime_type,content,sha256,created_by,updated_by)VALUES(gen_random_uuid(),'database',$1,'old attachment','notes.txt','text/plain','legacy bytes','synthetic',$2,$2);
 INSERT INTO favorites(user_id,database_id)VALUES($2,$1);`, pgx.QueryExecModeSimpleProtocol, db, actor); err != nil {
		t.Fatal(err)
	}
	var beforeHistory, beforeAudit, beforeVersions []byte
	if err = tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(h)ORDER BY id),'[]')FROM object_history h`).Scan(&beforeHistory); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(a)ORDER BY id)FROM audit_events a`).Scan(&beforeAudit); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(v)ORDER BY entity,object_id,version)FROM record_versions v`).Scan(&beforeVersions); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, p, schema); err != nil {
		t.Fatal(err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		query  string
		before []byte
	}{
		{`SELECT coalesce(jsonb_agg(to_jsonb(h)ORDER BY id),'[]')FROM object_history h`, beforeHistory},
		{`SELECT jsonb_agg(to_jsonb(a)ORDER BY id)FROM audit_events a`, beforeAudit},
		{`SELECT jsonb_agg(to_jsonb(v)ORDER BY entity,object_id,version)FROM record_versions v`, beforeVersions},
	} {
		var equal bool
		if err = tx.QueryRow(ctx, "SELECT("+item.query+")=$1::jsonb", item.before).Scan(&equal); err != nil || !equal {
			t.Fatal("historical data changed", item.query, err)
		}
	}
	var n int
	for _, table := range []string{"audit_events", "record_versions", "history_events"} {
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM pg_partition_tree($1::regclass)WHERE level=1`, table).Scan(&n); err != nil || n < 8 && table != "audit_events" {
			t.Fatal(table, n, err)
		}
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM catalog_search_documents`).Scan(&n); err != nil || n != 4 {
		t.Fatal("search backfill", n, err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM object_attachments WHERE content='legacy bytes'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("attachment lost", n, err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM audit_events_default`).Scan(&n); err != nil || n != 0 {
		t.Fatal("historical events not in month partitions", n, err)
	}
	// Direct SQL now increments versions and records history without a Go writer.
	if _, err = tx.Exec(ctx, `UPDATE servers SET description='direct maintenance SQL' WHERE id=$1`, server); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT version FROM servers WHERE id=$1`, server).Scan(&n); err != nil || n != 3 {
		t.Fatal("technical trigger", n, err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM record_versions WHERE entity='server' AND object_id=$1 AND version=3`, server).Scan(&n); err != nil || n != 1 {
		t.Fatal("direct SQL history missing", n, err)
	}
	// DEFAULT receives an unexpected date; maintenance moves it once without replaying notifications.
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,created_at)VALUES($1,'future-sample','2060-02-15')`, actor); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT maintain_catalog_partitions()`).Scan(&n); err != nil || n != 1 {
		t.Fatal("partition maintenance", n, err)
	}
	if err = tx.QueryRow(ctx, `SELECT maintain_catalog_partitions()`).Scan(&n); err != nil || n != 0 {
		t.Fatal("maintenance not idempotent", n, err)
	}
	var month time.Time
	if err = tx.QueryRow(ctx, `SELECT created_at FROM audit_events WHERE action='future-sample'`).Scan(&month); err != nil || month.Year() != 2060 {
		t.Fatal(month, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// A desktop connected before the upgrade cannot keep issuing old writes.
	blocked, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocked.Exec(ctx, "SET LOCAL application_name='CatalogDB'; SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err = blocked.Exec(ctx, "UPDATE servers SET description='old client overwrite'"); err == nil {
		t.Fatal("connected old client was not rejected")
	}
	_ = blocked.Rollback(ctx)
	if err = Migrate(ctx, p, schema); err != nil {
		t.Fatal("repeat migrate", err)
	}
	if err = migrateFS(ctx, p, schema, prior); err == nil {
		t.Fatal("old client accepted newer schema")
	}
}
