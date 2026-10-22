package application

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"strconv"
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

const catalogMatchCTE = `WITH attachment_search AS MATERIALIZED(SELECT entity,object_id,string_agg(search_text,' ') AS search_text FROM object_attachments WHERE cardinality($1::text[])>0 GROUP BY entity,object_id),eligible_db AS MATERIALIZED(
 SELECT d.id,d.instance_id,d.server_id FROM databases d
 LEFT JOIN instances i ON i.id=d.instance_id JOIN servers s ON s.id=coalesce(d.server_id,i.server_id)
 LEFT JOIN attachment_search sa ON sa.entity='server' AND sa.object_id=s.id
 LEFT JOIN attachment_search ia ON ia.entity='instance' AND ia.object_id=i.id
 LEFT JOIN attachment_search da ON da.entity='database' AND da.object_id=d.id
 LEFT JOIN backup_policies dp ON dp.database_id=d.id LEFT JOIN backup_policies ip ON ip.instance_id=d.instance_id
 WHERE ($2='' OR coalesce(nullif(d.dbms,''),i.dbms)=$2) AND($3='' OR d.status=$3) AND($4='' OR d.environment=$4)
 AND($5='' OR CASE WHEN $5='__empty__' THEN trim(coalesce(d.attributes->>'responsible',''))='' ELSE d.attributes->>'responsible'=$5 END)
 AND($6 IN('','any') OR($6='present' AND coalesce(dp.id,ip.id) IS NOT NULL) OR($6='missing' AND coalesce(dp.id,ip.id) IS NULL)
 OR($6='active' AND coalesce(dp.enabled,ip.enabled)) OR($6='disabled' AND NOT coalesce(dp.enabled,ip.enabled,true)))
 AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) term WHERE strpos(concat_ws(' ',s.search_text,sa.search_text,i.search_text,ia.search_text,d.search_text,da.search_text),lower(term))=0)
),eligible_instance AS MATERIALIZED(
 SELECT i.id,i.server_id FROM instances i JOIN servers s ON s.id=i.server_id
 LEFT JOIN attachment_search sa ON sa.entity='server' AND sa.object_id=s.id
 LEFT JOIN attachment_search ia ON ia.entity='instance' AND ia.object_id=i.id
 WHERE EXISTS(SELECT 1 FROM eligible_db d WHERE d.instance_id=i.id)
 OR($7 AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) term WHERE strpos(concat_ws(' ',s.search_text,sa.search_text,i.search_text,ia.search_text),lower(term))=0))
),eligible_server AS MATERIALIZED(
 SELECT s.id FROM servers s LEFT JOIN attachment_search sa ON sa.entity='server' AND sa.object_id=s.id WHERE EXISTS(SELECT 1 FROM eligible_instance i WHERE i.server_id=s.id)
 OR EXISTS(SELECT 1 FROM eligible_db d WHERE d.server_id=s.id)
 OR($7 AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) term WHERE strpos(concat_ws(' ',s.search_text,sa.search_text),lower(term))=0))
) `

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
	out := CatalogPage{Items: []Object{}, Children: map[string]ChildCounts{}, PageSize: 100}
	args, err := catalogMatchArgs(r.Criteria)
	if err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, catalogMatchCTE+`SELECT(SELECT count(*) FROM eligible_server),(SELECT count(*) FROM eligible_instance),(SELECT count(*) FROM eligible_db)`, args...).Scan(&out.Counts.Servers, &out.Counts.Instances, &out.Counts.Databases); err != nil {
		return out, importDatabaseError(err)
	}
	kind, eligible, condition := "server", "eligible_server", "true"
	switch r.ParentKind {
	case "":
		out.PageSize = 10
		out.Total = out.Counts.Servers
	case "server":
		kind, eligible, condition = "instance", "eligible_instance", "t.server_id=$8"
		args = append(args, r.ParentID)
	case "instance":
		kind, eligible, condition = "database", "eligible_db", "t.instance_id=$8"
		args = append(args, r.ParentID)
	case "oracle":
		kind, eligible, condition = "database", "eligible_db", "t.server_id=$8"
		args = append(args, r.ParentID)
	default:
		return out, errors.New("Неизвестная ветвь каталога")
	}
	t, _ := table(kind)
	if r.ParentKind != "" {
		if err = tx.QueryRow(ctx, catalogMatchCTE+"SELECT count(*) FROM "+t+" t JOIN "+eligible+" e ON e.id=t.id WHERE "+condition, args...).Scan(&out.Total); err != nil {
			return out, importDatabaseError(err)
		}
	}
	out.Page = boundedPage(r.Page, out.Total, out.PageSize)
	n := len(args)
	args = append(args, out.PageSize, out.Page*out.PageSize)
	// Scope conditions are selected above; values, search terms and pagination are bound parameters.
	query := catalogMatchCTE + "SELECT to_jsonb(t)-'search_text' FROM " + t + " t JOIN " + eligible + " e ON e.id=t.id WHERE " + condition + " ORDER BY lower(t.name),t.id LIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return out, importDatabaseError(err)
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return out, err
		}
		o, e := decodeObject(raw, kind)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, o)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	if kind == "database" {
		return out, nil
	}
	baseArgs, err := catalogMatchArgs(r.Criteria)
	if err != nil {
		return out, err
	}
	ids := objectIDs(out.Items)
	baseArgs = append(baseArgs, ids)
	var countsSQL string
	if kind == "server" {
		countsSQL = `SELECT s.id::text,(SELECT count(*) FROM eligible_instance i WHERE i.server_id=s.id),0::bigint,(SELECT count(*) FROM eligible_db d WHERE d.server_id=s.id) FROM servers s WHERE s.id=ANY($8::uuid[])`
	} else {
		countsSQL = `SELECT i.id::text,0::bigint,(SELECT count(*) FROM eligible_db d WHERE d.instance_id=i.id),0::bigint FROM instances i WHERE i.id=ANY($8::uuid[])`
	}
	rows, err = tx.Query(ctx, catalogMatchCTE+countsSQL, baseArgs...)
	if err != nil {
		return out, importDatabaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var c ChildCounts
		if err = rows.Scan(&id, &c.Instances, &c.Databases, &c.Oracle); err != nil {
			return out, err
		}
		out.Children[id] = c
	}
	return out, rows.Err()
}
