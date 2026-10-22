package application

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestGlobalCatalogSearchIncludesUnloadedBranchesAndKeepsANDOnOnePath(t *testing.T) {
	s, p, schema := catalogFixture(t)
	var last Object
	for n := 0; n < 12; n++ {
		o, err := s.SaveObject(Object{Kind: "server", Name: fmt.Sprintf("host-%02d", n), Address: fmt.Sprintf("10.0.0.%d", n+1)})
		if err != nil {
			t.Fatal(err)
		}
		last = o
	}
	instance, err := s.SaveObject(Object{Kind: "instance", Name: "pg", ParentID: last.ID, DBMS: "PostgreSQL"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Exec(context.Background(), "INSERT INTO "+pgx.Identifier{schema, "databases"}.Sanitize()+`(id,instance_id,name,environment,attributes,updated_by)SELECT gen_random_uuid(),$1,'sid-'||lpad(n::text,5,'0'),'Промышленный',jsonb_build_object('responsible','Пётр'),$2 FROM generate_series(1,1200)n`, instance.ID, s.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, err := s.BrowseCatalog(CatalogBrowseRequest{})
	if err != nil || len(root.Items) != 10 || root.Total != 12 || root.Counts.Databases != 1200 {
		t.Fatal(root, err)
	}
	for _, o := range root.Items {
		if o.ID == last.ID {
			t.Fatal("test target unexpectedly on initial page")
		}
	}
	f := CatalogCriteria{Query: "host-11 | sid-01200", DBMS: "PostgreSQL", Responsible: "Пётр"}
	root, err = s.BrowseCatalog(CatalogBrowseRequest{Criteria: f})
	if err != nil || len(root.Items) != 1 || root.Items[0].ID != last.ID || root.Counts.Databases != 1 {
		t.Fatal(root, err)
	}
	branch, err := s.BrowseCatalog(CatalogBrowseRequest{Criteria: f, ParentKind: "instance", ParentID: instance.ID})
	if err != nil || len(branch.Items) != 1 || branch.Items[0].Name != "sid-01200" {
		t.Fatal(branch, err)
	}
	object := branch.Items[0]
	context, err := s.GetObjectContext("database", object.ID)
	if err != nil || context.ServerPage != 1 || context.BranchPages[instance.ID] != 11 || len(context.Catalog.Databases) != 1 {
		t.Fatal(context, err)
	}
	refs, err := s.SelectCatalogResult(f)
	if err != nil || len(refs) != 1 || refs[0].ID != object.ID {
		t.Fatal(refs, err)
	}
	refs, err = s.SelectCatalogResult(CatalogCriteria{})
	if err != nil || len(refs) != 1200 {
		t.Fatal(len(refs), err)
	}
	outside, err := s.CountSelectionOutside(f, []string{object.ID, refs[0].ID})
	if err != nil || outside != 1 {
		t.Fatal(outside, err)
	}
	noSiblingMatch, err := s.BrowseCatalog(CatalogBrowseRequest{Criteria: CatalogCriteria{Query: "sid-00001 | sid-00002"}})
	if err != nil || noSiblingMatch.Total != 0 || noSiblingMatch.Counts.Databases != 0 {
		t.Fatal(noSiblingMatch, err)
	}
	ordinary, err := s.BrowseCatalog(CatalogBrowseRequest{ParentKind: "instance", ParentID: instance.ID, Page: 11})
	if err != nil || len(ordinary.Items) != 100 || ordinary.Total != 1200 {
		t.Fatal(ordinary, err)
	}
	options, err := s.FindObjects("database", "host-11 | sid-01200", 0)
	if err != nil || options.Total != 1 || options.Items[0].Object.ID != object.ID {
		t.Fatal(options, err)
	}
	// Oracle is a sibling group, never an invented instance.
	oracle, err := s.SaveObject(Object{Kind: "database", DBMS: "Oracle Database", Name: "literal%_Русская", ParentID: last.ID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.BrowseCatalog(CatalogBrowseRequest{Criteria: CatalogCriteria{Query: "%_РУССКАЯ", DBMS: "Oracle Database"}})
	if err != nil || result.Total != 1 || result.Children[last.ID].Oracle != 1 || result.Children[last.ID].Instances != 0 {
		t.Fatal(result, err)
	}
	direct, err := s.BrowseCatalog(CatalogBrowseRequest{ParentKind: "oracle", ParentID: last.ID, Criteria: CatalogCriteria{Query: "%_русская"}})
	if err != nil || len(direct.Items) != 1 || direct.Items[0].ID != oracle.ID {
		t.Fatal(direct, err)
	}
	file, err := s.ExportCatalogResult(CatalogCriteria{Query: "sid-01200"})
	if err != nil || file.Size == 0 {
		t.Fatal(file, err)
	}
}
func TestCatalogFiltersUseEffectivePoliciesBeyondLoadedObjects(t *testing.T) {
	s, _, _ := catalogFixture(t)
	_, instance, dbs := workflowBranch(t, s)
	_, err := s.SavePolicy(Policy{Kind: "instance", ObjectID: instance.ID, Name: "Inherited", Enabled: true, BackupType: "Локальное", Scheduler: "cron", Schedule: "02:00", Storage: "/backup", RetentionDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SavePolicy(Policy{Kind: "database", ObjectID: dbs[0].ID, Name: "Disabled"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		policy string
		count  int64
	}{{"active", 1}, {"disabled", 1}, {"missing", 0}, {"present", 2}} {
		got, err := s.BrowseCatalog(CatalogBrowseRequest{Criteria: CatalogCriteria{Policy: test.policy}})
		if err != nil || got.Counts.Databases != test.count {
			t.Fatal(test, got, err)
		}
	}
	policies, err := s.GetPolicyPage("host | Disabled", 0)
	if err != nil || policies.Total != 1 || policies.Paths[policies.Items[0].ID] != "host / pg / one" {
		t.Fatal(policies, err)
	}
}
func TestLargeCatalogPageRemainsBounded(t *testing.T) {
	s, p, schema := catalogFixture(t)
	server, instance, _ := workflowBranch(t, s)
	_, err := p.Exec(context.Background(), "INSERT INTO "+pgx.Identifier{schema, "databases"}.Sanitize()+`(id,instance_id,name,updated_by)SELECT gen_random_uuid(),$1,'large-'||lpad(n::text,6,'0'),$2 FROM generate_series(1,20000)n`, instance.ID, s.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.GetCatalogWorkspace()
	if err != nil || len(initial.Root.Items) != 1 || initial.Root.Items[0].ID != server.ID || initial.Root.Counts.Databases != 20002 {
		t.Fatal(initial.Root, err)
	}
	page, err := s.BrowseCatalog(CatalogBrowseRequest{ParentKind: "instance", ParentID: instance.ID})
	if err != nil || len(page.Items) != 100 || page.Total != 20002 {
		t.Fatal(len(page.Items), page.Total, err)
	}
	search, err := s.BrowseCatalog(CatalogBrowseRequest{Criteria: CatalogCriteria{Query: "large-020000"}})
	if err != nil || search.Counts.Databases != 1 || search.Total != 1 {
		t.Fatal(search, err)
	}
	if _, err = s.SelectCatalogResult(CatalogCriteria{}); err == nil {
		t.Fatal("unbounded bulk selection accepted")
	}
}
