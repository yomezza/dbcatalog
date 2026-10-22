package application

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/spreadsheet"
)

func performanceWorkbook(size int) spreadsheet.Workbook {
	wb := spreadsheet.Workbook{Columns: 21, Rows: make([]spreadsheet.Row, size)}
	for i := range wb.Rows {
		wb.Rows[i] = spreadsheet.Row{Number: i + 2}
		copy(wb.Rows[i].Values[:], []string{fmt.Sprintf("db-%05d", i), "pg-main", "Эксплуатируется", "Промышленный", "host", "PostgreSQL", "17", "SSH", "отсутствует", "127.0.0.1", "ЦОД", "ИС", "Организация", "Руководитель", "Ответственный", "Linux", "1", "Linux", "Назначение", "Мониторинг и контроль;Резервное копирование", "Нет"})
	}
	return wb
}

type queryCounter struct{ total, lookups atomic.Int64 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.total.Add(1)
	if strings.Contains(data.SQL, "lookup_values") {
		c.lookups.Add(1)
	}
	return ctx
}
func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func traceQueries(t testing.TB, s *Service) *queryCounter {
	counter := &queryCounter{}
	setQueryTracer(t, s, counter)
	return counter
}
func setQueryTracer(t testing.TB, s *Service, tracer pgx.QueryTracer) {
	t.Helper()
	config := s.pool.Config()
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	previous := s.pool
	s.pool = pool
	t.Cleanup(previous.Close)
}
func BenchmarkImportPlan600(b *testing.B) {
	s, _, _ := catalogFixture(b)
	counter := traceQueries(b, s)
	wb := performanceWorkbook(600)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, err := s.readTransaction(ctx, true, false)
		if err != nil {
			b.Fatal(err)
		}
		plan, err := s.buildImportPlan(ctx, tx, wb)
		tx.Rollback(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if plan.preview.Valid != len(wb.Rows) {
			b.Fatal("unexpected invalid rows")
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(counter.lookups.Load())/float64(b.N), "lookup-queries/op")
}

func TestImportValidationQueryBudgetAndFreshDictionaries(t *testing.T) {
	s, p, schema := catalogFixture(t)
	counter := traceQueries(t, s)
	ctx := context.Background()
	for _, size := range []int{1, 600} {
		counter.lookups.Store(0)
		tx, err := s.readTransaction(ctx, true, false)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := s.buildImportPlan(ctx, tx, performanceWorkbook(size))
		tx.Rollback(ctx)
		if err != nil || plan.preview.Valid != size {
			t.Fatal("unexpected preview", err)
		}
		if got := counter.lookups.Load(); got > 2 {
			t.Fatalf("%d rows required %d lookup queries", size, got)
		}
	}
	// A cached lookup must not survive into a new operation.
	wb := performanceWorkbook(1)
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.buildImportPlan(ctx, tx, wb)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s.pendingImport = plan
	if _, err = p.Exec(ctx, "DELETE FROM "+pgx.Identifier{schema, "lookup_values"}.Sanitize()+" WHERE category='administration' AND value='Мониторинг и контроль'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyExcel(plan.preview.Token, false, false); err == nil {
		t.Fatal("stale dictionary accepted during apply")
	}
	catalog, err := s.GetCatalog()
	if err != nil || len(catalog.Servers) > 0 {
		t.Fatal("failed validation partially wrote catalog", err)
	}
	tx, err = s.readTransaction(ctx, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	next, err := s.buildImportPlan(ctx, tx, wb)
	if err != nil {
		t.Fatal(err)
	}
	if next.preview.Valid != 0 {
		t.Fatal("next preview reused stale dictionary")
	}
}
