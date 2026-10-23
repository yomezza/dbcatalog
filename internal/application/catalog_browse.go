package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

type CatalogCounts struct {
	Servers   int64 `json:"servers"`
	Instances int64 `json:"instances"`
	Databases int64 `json:"databases"`
}
type ChildCounts struct {
	Instances int64 `json:"instances"`
	Databases int64 `json:"databases"`
	Oracle    int64 `json:"oracle"`
}
type CatalogBrowseRequest struct {
	Criteria   CatalogCriteria `json:"criteria"`
	ParentKind string          `json:"parentKind"`
	ParentID   string          `json:"parentId"`
	Page       int             `json:"page"`
}
type CatalogPage struct {
	Items    []Object               `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
	Counts   CatalogCounts          `json:"counts"`
	Children map[string]ChildCounts `json:"children"`
}

const catalogMatchCTE = `WITH matched AS MATERIALIZED(SELECT * FROM api_catalog_matches($1,$2,$3,$4,$5,$6,$7)),
eligible_server AS(SELECT id FROM matched WHERE entity='server'),
eligible_instance AS(SELECT id,parent_id AS server_id FROM matched WHERE entity='instance'),
eligible_db AS(SELECT d.id,d.instance_id,d.server_id FROM matched m JOIN databases d ON d.id=m.id WHERE m.entity='database') `

func catalogMatchArgs(f CatalogCriteria) ([]any, error) {
	if len(f.Query) > 2000 {
		return nil, errors.New("Слишком длинный поисковый запрос")
	}
	switch f.Policy {
	case "", "any", "active", "disabled", "present", "missing":
	default:
		return nil, errors.New("Неизвестный фильтр политики РК")
	}
	terms := []string{}
	seen := map[string]bool{}
	for _, t := range strings.Split(f.Query, "|") {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			terms = append(terms, t)
			seen[t] = true
		}
	}
	noFields := f.DBMS == "" && f.Status == "" && f.Environment == "" && f.Responsible == "" && (f.Policy == "" || f.Policy == "any")
	return []any{terms, f.DBMS, f.Status, f.Environment, f.Responsible, f.Policy, noFields}, nil
}
func (s *Service) BrowseCatalog(r CatalogBrowseRequest) (CatalogPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return CatalogPage{}, err
	}
	defer tx.Rollback(ctx)
	return browseCatalog(ctx, tx, r)
}
func browseCatalog(ctx context.Context, tx pgx.Tx, r CatalogBrowseRequest) (CatalogPage, error) {
	out := CatalogPage{}
	if _, err := catalogMatchArgs(r.Criteria); err != nil {
		return out, err
	}
	raw, err := json.Marshal(r.Criteria)
	if err != nil {
		return out, err
	}
	var result []byte
	if err = tx.QueryRow(ctx, `SELECT api_catalog_page($1,$2,$3,$4)`, raw, r.ParentKind, nullableID(r.ParentID), r.Page).Scan(&result); err != nil {
		return out, importDatabaseError(err)
	}
	err = json.Unmarshal(result, &out)
	return out, err
}
