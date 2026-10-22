package application

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func seedDeletionBranch(t *testing.T, s *Service, name string) (Object, Object, []Object, []Policy) {
	t.Helper()
	server, err := s.SaveObject(Object{Kind: "server", Name: name})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := s.SaveObject(Object{Kind: "instance", Name: "pg", DBMS: "PostgreSQL", ParentID: server.ID})
	if err != nil {
		t.Fatal(err)
	}
	dbs := []Object{}
	for _, name := range []string{"first", "second"} {
		db, err := s.SaveObject(Object{Kind: "database", Name: name, ParentID: instance.ID})
		if err != nil {
			t.Fatal(err)
		}
		dbs = append(dbs, db)
	}
	policies := []Policy{}
	for _, p := range []Policy{{Kind: "instance", ObjectID: instance.ID, Name: "Общая"}, {Kind: "database", ObjectID: dbs[0].ID, Name: "Своя"}} {
		saved, err := s.SavePolicy(p)
		if err != nil {
			t.Fatal(err)
		}
		policies = append(policies, saved)
	}
	for _, o := range []Object{server, instance, dbs[0]} {
		if err = s.SetFavorite(o.Kind, o.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	return server, instance, dbs, policies
}
func TestCascadeDeletionIsAtomicAndPreservesHistory(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	root, _, dbs, _ := seedDeletionBranch(t, s, "delete-me")
	other, _, _, _ := seedDeletionBranch(t, s, "keep-me")
	preview, err := s.PreviewDeletion("server", root.ID, root.Version)
	if err != nil || preview.Servers != 1 || preview.Instances != 1 || preview.Databases != 2 || preview.Policies != 2 {
		t.Fatal("incorrect preview", preview, err)
	}
	q := pgx.Identifier{schema}.Sanitize()
	_, err = p.Exec(ctx, `CREATE FUNCTION `+q+`.block_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture rejection'; END $$; CREATE TRIGGER block_delete BEFORE DELETE ON `+q+`.servers FOR EACH ROW EXECUTE FUNCTION `+q+`.block_delete()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteObjectConfirmed("server", root.ID, root.Version, preview.Fingerprint); err == nil {
		t.Fatal("delete ignored root rejection")
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Servers) != 2 || len(c.Instances) != 2 || len(c.Databases) != 4 {
		t.Fatal("partial delete persisted", err)
	}
	policies, err := s.ListPolicies()
	if err != nil || len(policies) != 4 {
		t.Fatal("policy deletion did not roll back", err)
	}
	fav, err := s.ListFavorites()
	if err != nil || len(fav) != 6 {
		t.Fatal("favorites did not roll back", err)
	}
	var deleted int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM "+q+`.object_history WHERE action='delete'`).Scan(&deleted); err != nil || deleted != 0 {
		t.Fatal("failed operation left history", err)
	}
	if _, err = p.Exec(ctx, `DROP TRIGGER block_delete ON `+q+`.servers`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteObjectConfirmed("server", root.ID, root.Version, preview.Fingerprint); err != nil {
		t.Fatal(err)
	}
	c, err = s.GetCatalog()
	if err != nil || len(c.Servers) != 1 || c.Servers[0].ID != other.ID || len(c.Instances) != 1 || len(c.Databases) != 2 {
		t.Fatal("wrong subtree deleted", err)
	}
	policies, err = s.ListPolicies()
	if err != nil || len(policies) != 2 {
		t.Fatal("policies outside subtree changed", err)
	}
	fav, err = s.ListFavorites()
	if err != nil || len(fav) != 3 {
		t.Fatal("favorites outside subtree changed", err)
	}
	h, err := s.GetHistory("database", dbs[0].ID)
	if err != nil || len(h) != 2 || h[0].Action != "delete" || h[1].Action != "create" {
		t.Fatal("child history lost", err)
	}
	if err = p.QueryRow(ctx, "SELECT count(*) FROM "+q+`.object_history WHERE action='delete'`).Scan(&deleted); err != nil || deleted != 6 {
		t.Fatal("each deleted object must have history", deleted, err)
	}
}
func TestDeletionPreviewDetectsNewChildrenAndPolicyEdits(t *testing.T) {
	s, _, _ := catalogFixture(t)
	server, instance, _, policies := seedDeletionBranch(t, s, "host")
	preview, err := s.PreviewDeletion("instance", instance.ID, instance.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveObject(Object{Kind: "database", Name: "new-child", ParentID: instance.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteObjectConfirmed("instance", instance.ID, instance.Version, preview.Fingerprint); err == nil {
		t.Fatal("unreviewed new child deleted")
	}
	preview, err = s.PreviewDeletion("instance", instance.ID, instance.Version)
	if err != nil {
		t.Fatal(err)
	}
	policy := policies[0]
	policy.Description = "concurrent edit"
	if _, err = s.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteObjectConfirmed("instance", instance.ID, instance.Version, preview.Fingerprint); err == nil {
		t.Fatal("unreviewed policy edit deleted")
	}
	preview, err = s.PreviewDeletion("instance", instance.ID, instance.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteObjectConfirmed("instance", instance.ID, instance.Version, ""); err == nil {
		t.Fatal("bridge permits missing confirmation snapshot")
	}
	if _, err = s.DeleteObjectConfirmed("instance", instance.ID, instance.Version, preview.Fingerprint); err != nil {
		t.Fatal(err)
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Servers) != 1 || c.Servers[0].ID != server.ID || len(c.Instances)+len(c.Databases) != 0 {
		t.Fatal("instance delete affected parent", err)
	}
}
func TestClearCatalogPreservesUsersSettingsDictionariesAndAudit(t *testing.T) {
	s, p, schema := catalogFixture(t)
	_, _, _, _ = seedDeletionBranch(t, s, "one")
	_, _, _, _ = seedDeletionBranch(t, s, "two")
	ctx := context.Background()
	q := pgx.Identifier{schema}.Sanitize()
	beforeConfig, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	var users, lookups, created int
	for _, tc := range []struct {
		query string
		dst   *int
	}{{"SELECT count(*) FROM " + q + ".users", &users}, {"SELECT count(*) FROM " + q + ".lookup_values", &lookups}, {"SELECT count(*) FROM " + q + ".object_history WHERE action='create'", &created}} {
		if err = p.QueryRow(ctx, tc.query).Scan(tc.dst); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := s.PreviewClearCatalog()
	if err != nil || preview.Servers != 2 || preview.Databases != 4 || preview.Policies != 4 {
		t.Fatal("clear preview", preview, err)
	}
	if _, err = s.ClearCatalog(preview.Fingerprint, "yes"); err == nil {
		t.Fatal("clear without exact second confirmation")
	}
	if _, err = s.ClearCatalog(preview.Fingerprint, "УДАЛИТЬ КАТАЛОГ"); err != nil {
		t.Fatal(err)
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Servers)+len(c.Instances)+len(c.Databases) != 0 {
		t.Fatal("catalog not cleared", err)
	}
	policies, err := s.ListPolicies()
	if err != nil || len(policies) != 0 {
		t.Fatal("policies retained", err)
	}
	favorites, err := s.ListFavorites()
	if err != nil || len(favorites) != 0 {
		t.Fatal("stale favorites retained", err)
	}
	for _, tc := range []struct {
		query string
		want  int
	}{{"SELECT count(*) FROM " + q + ".users", users}, {"SELECT count(*) FROM " + q + ".lookup_values", lookups}, {"SELECT count(*) FROM " + q + ".object_history WHERE action='create'", created}, {"SELECT count(*) FROM " + q + ".audit_events WHERE action='catalog_clear'", 1}} {
		var n int
		if err = p.QueryRow(ctx, tc.query).Scan(&n); err != nil || n != tc.want {
			t.Fatal("protected data changed or audit missing", n, tc.want, err)
		}
	}
	afterConfig, err := os.ReadFile(s.path)
	if err != nil || string(beforeConfig) != string(afterConfig) {
		t.Fatal("connection settings changed", err)
	}
}
func TestViewerCannotPreviewOrPerformCatalogDeletion(t *testing.T) {
	s, _, _ := catalogFixture(t)
	server, _, _, _ := seedDeletionBranch(t, s, "host")
	branch, err := s.PreviewDeletion("server", server.ID, server.Version)
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.PreviewClearCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Register("viewer", "long-enough-password", false); err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Login == "viewer" {
			if err = s.UpdateUser(u.ID, "viewer", "active"); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err = s.Login("viewer", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewClearCatalog(); err == nil {
		t.Fatal("viewer can preview admin deletion")
	}
	if _, err = s.ClearCatalog(all.Fingerprint, "УДАЛИТЬ КАТАЛОГ"); err == nil {
		t.Fatal("viewer can clear")
	}
	if _, err = s.DeleteObjectConfirmed("server", server.ID, server.Version, branch.Fingerprint); err == nil {
		t.Fatal("viewer can delete")
	}
	if _, err = s.GetAudit(AuditFilter{}); err == nil {
		t.Fatal("viewer can read audit")
	}
}
