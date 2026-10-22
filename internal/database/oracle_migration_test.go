package database

import (
	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/security"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestOracleMigrationPreservesDataPoliciesFavoritesAndRejectsAmbiguity(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		name := "preserves"
		if duplicate {
			name = "ambiguous_rollback"
		}
		t.Run(name, func(t *testing.T) {
			ctx, p := testPool(t)
			schema := testSchema(t, ctx, p)
			old := fstest.MapFS{}
			files, _ := fs.ReadDir(migrationFiles, "migrations")
			for _, f := range files {
				if f.Name() < "006" {
					raw, _ := migrationFiles.ReadFile("migrations/" + f.Name())
					old["migrations/"+f.Name()] = &fstest.MapFile{Data: raw}
				}
			}
			if err := migrateFS(ctx, p, schema, old); err != nil {
				t.Fatal(err)
			}
			q := pgx.Identifier{schema}.Sanitize()
			uid, sid, iid, iid2, d1, d2, pid, own, d3 := security.ID(), security.ID(), security.ID(), security.ID(), security.ID(), security.ID(), security.ID(), security.ID(), security.ID()
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			tx.Exec(ctx, "SET LOCAL search_path="+q)
			statements := []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO users(id,login,password_hash,role,state)VALUES($1,'admin','hash','admin','active')`, []any{uid}},
				{`INSERT INTO servers(id,name,updated_by)VALUES($1,'host',$2)`, []any{sid, uid}},
				{`INSERT INTO instances(id,server_id,name,dbms,dbms_version,description,updated_by,port)VALUES($1,$2,'default','Oracle Database','19c','legacy metadata',$3,1521),($4,$2,'other','Oracle Database','21c','other metadata',$3,1522)`, []any{iid, sid, uid, iid2}},
				{`INSERT INTO databases(id,instance_id,name,purpose,updated_by)VALUES($1,$2,'ONE','keep purpose',$3),($4,$5,$6,'second',$3)`, []any{d1, iid, uid, d2, iid2, map[bool]string{false: "TWO", true: "ONE"}[duplicate]}},
				{`INSERT INTO databases(id,instance_id,name,updated_by)VALUES($1,$2,'THREE',$3)`, []any{d3, iid, uid}},
				{`INSERT INTO backup_policies(id,instance_id,name,enabled,backup_type,scheduler,schedule,storage,retention_days,updated_by)VALUES($1,$2,'shared',true,'Локальное','cron','daily','/backup',7,$3)`, []any{pid, iid, uid}},
				{`INSERT INTO backup_policies(id,database_id,name,updated_by)VALUES($1,$2,'own disabled',$3)`, []any{own, d2, uid}},
				{`INSERT INTO favorites(user_id,instance_id)VALUES($1,$2)`, []any{uid, iid}},
			}
			for _, stmt := range statements {
				if _, err = tx.Exec(ctx, stmt.sql, stmt.args...); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = Migrate(ctx, p, schema)
			var count int
			if duplicate {
				if err == nil || !strings.Contains(PublicError(err), "одинаковые SID") {
					t.Fatal(err)
				}
				p.QueryRow(ctx, "SELECT count(*) FROM "+q+".instances").Scan(&count)
				if count != 2 {
					t.Fatal("migration lost legacy instances")
				}
				p.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name='databases' AND column_name='server_id'`, schema).Scan(&count)
				if count != 0 {
					t.Fatal("partial DDL persisted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p.QueryRow(ctx, "SELECT count(*) FROM "+q+".instances").Scan(&count)
			if count != 0 {
				t.Fatal("Oracle instances left")
			}
			var parent, dbms, version, purpose string
			var objectVersion int
			err = p.QueryRow(ctx, "SELECT server_id::text,dbms,dbms_version,purpose,version FROM "+q+".databases WHERE id=$1", d1).Scan(&parent, &dbms, &version, &purpose, &objectVersion)
			if err != nil || parent != sid || dbms != "Oracle Database" || version != "19c" || purpose != "keep purpose" || objectVersion != 2 {
				t.Fatal(parent, dbms, version, purpose, objectVersion, err)
			}
			var port int
			if err = p.QueryRow(ctx, "SELECT port FROM "+q+".databases WHERE id=$1", d1).Scan(&port); err != nil || port != 1521 {
				t.Fatal("Oracle port not retained", port, err)
			}
			var enabled bool
			err = p.QueryRow(ctx, "SELECT enabled FROM "+q+".backup_policies WHERE database_id=$1", d1).Scan(&enabled)
			if err != nil || !enabled {
				t.Fatal("inherited policy not materialized", err)
			}
			if err = p.QueryRow(ctx, "SELECT enabled FROM "+q+".backup_policies WHERE database_id=$1", d3).Scan(&enabled); err != nil || !enabled {
				t.Fatal("shared policy not copied to all databases", err)
			}
			err = p.QueryRow(ctx, "SELECT enabled FROM "+q+".backup_policies WHERE id=$1 AND database_id=$2", own, d2).Scan(&enabled)
			if err != nil || enabled {
				t.Fatal("explicit disabled policy changed", err)
			}
			p.QueryRow(ctx, "SELECT count(*) FROM "+q+".favorites WHERE user_id=$1 AND database_id=$2", uid, d1).Scan(&count)
			if count != 1 {
				t.Fatal("favorite not migrated")
			}
			p.QueryRow(ctx, "SELECT count(*) FROM "+q+".object_history WHERE entity='instance' AND before_state->>'description'='legacy metadata'").Scan(&count)
			if count != 1 {
				t.Fatal("instance metadata lost")
			}
			if err = Migrate(ctx, p, schema); err != nil {
				t.Fatal("repeat migration", err)
			}
		})
	}
}
