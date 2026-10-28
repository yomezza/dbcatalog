package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/downloads"
	"github.com/yomezza/test/internal/reports"
	"github.com/yomezza/test/internal/security"
)

const MaxPDFDatabases = 500

type PDFOptions struct {
	Layout          string `json:"layout"`
	IncludePolicies bool   `json:"includePolicies"`
}

func (s *Service) ExportObjectPDFWithOptions(kind, id string, options PDFOptions) (downloads.Entry, error) {
	return s.exportPDF(kind, []string{id}, options, false)
}

func (s *Service) ExportSelectionPDF(ids []string, options PDFOptions) (downloads.Entry, error) {
	return s.exportPDF("database", ids, options, true)
}

func (s *Service) exportPDF(kind string, ids []string, options PDFOptions, batch bool) (downloads.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if options.Layout != "detailed" && options.Layout != "compact" {
		return downloads.Entry{}, errors.New("Выберите подробный или краткий вид PDF")
	}
	if len(ids) == 0 || len(ids) > MaxPDFDatabases {
		return downloads.Entry{}, fmt.Errorf("Для одного PDF выберите от 1 до %d БД", MaxPDFDatabases)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return downloads.Entry{}, errors.New("Выбор содержит пустой или повторяющийся ID")
		}
		seen[id] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store, err := s.downloadStore(ctx)
	if err != nil {
		return downloads.Entry{}, err
	}
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return downloads.Entry{}, err
	}
	defer tx.Rollback(ctx)
	now := time.Now()
	var items []reports.Passport
	if batch {
		items, err = loadSelectionPassports(ctx, tx, ids, s.user.Login, now)
	} else {
		var item reports.Passport
		item, err = buildObjectPassport(ctx, tx, kind, ids[0], s.user.Login, now)
		items = []reports.Passport{item}
	}
	if err != nil {
		return downloads.Entry{}, err
	}
	if !options.IncludePolicies {
		for i := range items {
			item := &items[i]
			sections := []reports.Section{}
			for _, section := range item.Sections {
				if section.Title != "Резервное копирование" {
					sections = append(sections, section)
				}
			}
			item.Sections = sections
			summary := []reports.Field{}
			for _, field := range item.Summary {
				if field.Label != "Политика РК" {
					summary = append(summary, field)
				}
			}
			item.Summary = summary
		}
	}
	data, err := reports.RenderPassports(items, options.Layout)
	if err != nil {
		if errors.Is(err, reports.ErrCompactOverflow) {
			return downloads.Entry{}, err
		}
		return downloads.Entry{}, errors.New("Не удалось сформировать PDF-паспорта")
	}
	// Record a single batch event, not hundreds of independently committed exports.
	auditKind, auditID := kind, ids[0]
	auditName := items[0].Name
	if batch {
		auditKind = "export"
		auditID = security.ID()
		auditName = fmt.Sprintf("Выбранные БД: %d", len(items))
	}
	details, _ := json.Marshal(map[string]any{"objectName": auditName, "count": len(items), "layout": options.Layout, "includePolicies": options.IncludePolicies})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id,details) VALUES($1,'pdf_export',$2,$3,$4)`, s.user.ID, auditKind, auditID, details); err != nil {
		return downloads.Entry{}, importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return downloads.Entry{}, importDatabaseError(err)
	}
	name := "Выбранные-БД"
	category := "PDF-паспорта выбранных БД"
	if !batch {
		name = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
				return '_'
			}
			return r
		}, items[0].Name)
		name = string([]rune(name)[:min(60, len([]rune(name)))])
		category = "PDF-паспорт объекта"
	}
	name = "CatalogDB-Паспорт-" + strings.Trim(name, " .") + "-" + now.Format("2006-01-02-150405") + ".pdf"
	entry, err := store.Add(name, category, data)
	if err != nil {
		return downloads.Entry{}, errors.New("Не удалось сохранить PDF в локальные загрузки")
	}
	return entry, nil
}

// Two queries for the entire selection: parents and effective policies are
// resolved within one authorized repeatable-read snapshot, without UI pages.
func loadSelectionPassports(ctx context.Context, tx pgx.Tx, ids []string, author string, now time.Time) ([]reports.Passport, error) {
	rows, err := tx.Query(ctx, `SELECT to_jsonb(d)-'search_text',to_jsonb(i)-'search_text',to_jsonb(s)-'search_text'
	 FROM databases d LEFT JOIN instances i ON i.id=d.instance_id JOIN servers s ON s.id=coalesce(d.server_id,i.server_id)
	 WHERE d.id=ANY($1::uuid[]) ORDER BY lower(s.name),s.id,lower(i.name),i.id,lower(d.name),d.id`, ids)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	contexts := []ObjectContext{}
	instanceIDs := []string{}
	for rows.Next() {
		var dr, ir, sr []byte
		if err = rows.Scan(&dr, &ir, &sr); err != nil {
			rows.Close()
			return nil, importDatabaseError(err)
		}
		d, e := decodeObject(dr, "database")
		if e != nil {
			rows.Close()
			return nil, e
		}
		s, e := decodeObject(sr, "server")
		if e != nil {
			rows.Close()
			return nil, e
		}
		c := ObjectContext{Object: d, Catalog: Catalog{Servers: []Object{s}, Databases: []Object{d}}, Path: s.Name, Policies: []Policy{}}
		if len(ir) > 0 {
			i, e := decodeObject(ir, "instance")
			if e != nil {
				rows.Close()
				return nil, e
			}
			c.Catalog.Instances = []Object{i}
			c.Path += " / " + i.Name
			instanceIDs = append(instanceIDs, i.ID)
		} else {
			c.Path += " / Oracle Database"
		}
		c.Path += " / " + d.Name
		contexts = append(contexts, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, importDatabaseError(err)
	}
	if len(contexts) != len(ids) {
		return nil, errors.New("Часть выбранных БД удалена или недоступна. Обновите выбор; PDF не создан")
	}
	type policyTarget struct{ kind, id string }
	policies := map[policyTarget]Policy{}
	rows, err = tx.Query(ctx, `SELECT to_jsonb(p) FROM backup_policies p WHERE database_id=ANY($1::uuid[]) OR instance_id=ANY($2::uuid[])`, ids, instanceIDs)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, importDatabaseError(err)
		}
		p, e := decodePolicy(raw)
		if e != nil {
			rows.Close()
			return nil, e
		}
		policies[policyTarget{p.Kind, p.ObjectID}] = p
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, importDatabaseError(err)
	}
	out := make([]reports.Passport, 0, len(contexts))
	for _, c := range contexts {
		if p, ok := policies[policyTarget{"instance", c.Object.ParentID}]; ok && c.Object.DBMS != "Oracle Database" {
			c.Policies = append(c.Policies, p)
		}
		if p, ok := policies[policyTarget{"database", c.Object.ID}]; ok {
			c.Policies = append(c.Policies, p)
		}
		out = append(out, passportFromContext(c, author, now))
	}
	return out, nil
}
