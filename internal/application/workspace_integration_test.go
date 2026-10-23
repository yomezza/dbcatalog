package application

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
	"time"
)

type workspaceBarrier struct {
	started, proceed chan struct{}
	once             sync.Once
}

func (b *workspaceBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "lookup_values") || strings.Contains(data.SQL, "api_lookups") {
		b.once.Do(func() {
			close(b.started)
			select {
			case <-b.proceed:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}
func (b *workspaceBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func TestWorkspaceUsesOneSnapshotAcrossConcurrentCatalogChanges(t *testing.T) {
	s, p, schema := catalogFixture(t)
	_, _, dbs := workflowBranch(t, s)
	old := dbs[0]
	old.Attributes.Responsible = "Иван"
	var err error
	old, err = s.SaveObject(old)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := s.SavePolicy(Policy{Kind: "database", ObjectID: old.ID, Name: "Прежняя политика"})
	if err != nil {
		t.Fatal(err)
	}
	barrier := &workspaceBarrier{started: make(chan struct{}), proceed: make(chan struct{})}
	setQueryTracer(t, s, barrier)
	released := false
	defer func() {
		if !released {
			close(barrier.proceed)
		}
	}()
	type result struct {
		workspace Workspace
		err       error
	}
	done := make(chan result, 1)
	go func() { w, err := s.GetWorkspace(); done <- result{w, err} }()
	select {
	case <-barrier.started:
	case <-time.After(10 * time.Second):
		t.Fatal("workspace query did not reach barrier")
	}
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "databases"}.Sanitize()+" SET attributes=jsonb_set(attributes,'{responsible}','\"Пётр\"'),version=version+1 WHERE id=$1", old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "backup_policies"}.Sanitize()+" SET name='Новая политика',version=version+1 WHERE id=$1", policy.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO "+pgx.Identifier{schema, "favorites"}.Sanitize()+"(user_id,database_id)VALUES($1,$2)", s.user.ID, old.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	close(barrier.proceed)
	released = true
	var got result
	select {
	case got = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("workspace did not finish")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	if len(got.workspace.Policies) != 1 || got.workspace.Policies[0].Name != "Прежняя политика" || len(got.workspace.Favorites) != 0 {
		t.Fatal("mixed workspace snapshots")
	}
	for _, db := range got.workspace.Catalog.Databases {
		if db.ID == old.ID && db.Attributes.Responsible != "Иван" {
			t.Fatal("catalog snapshot changed")
		}
	}
	for _, v := range got.workspace.Lookups["responsible"] {
		if v == "Пётр" {
			t.Fatal("lookups escaped the catalog snapshot")
		}
	}
	next, err := s.GetWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if next.Policies[0].Name != "Новая политика" || len(next.Favorites) != 1 {
		t.Fatal("next operation reused an old snapshot")
	}
}
func TestWorkspaceChecksCurrentPermissionsAndReturnsNoPartialData(t *testing.T) {
	s, p, schema := catalogFixture(t)
	workflowBranch(t, s)
	if _, err := p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET role='viewer' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	w, err := s.GetWorkspace()
	if err != nil || len(w.Catalog.Databases) != 2 {
		t.Fatal("viewer cannot read workspace", err)
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET state='blocked' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	w, err = s.GetWorkspace()
	if err == nil || len(w.Catalog.Servers) > 0 || len(w.Lookups) > 0 || len(w.Favorites) > 0 || len(w.Policies) > 0 {
		t.Fatal("blocked user received workspace data")
	}
}
