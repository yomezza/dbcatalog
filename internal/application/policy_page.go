package application

import (
	"context"
	"time"
)

type PolicyPage struct {
	Items []Policy          `json:"items"`
	Paths map[string]string `json:"paths"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
}

const policyPageCTE = `WITH policies AS(
 SELECT p.*,concat_ws(' / ',s.name,CASE WHEN p.database_id IS NULL THEN i.name ELSE coalesce(di.name,'Oracle Database') END,d.name) AS target_path
 FROM backup_policies p LEFT JOIN instances i ON i.id=p.instance_id LEFT JOIN databases d ON d.id=p.database_id LEFT JOIN instances di ON di.id=d.instance_id
 JOIN servers s ON s.id=coalesce(i.server_id,d.server_id,di.server_id)
),matched AS(
 SELECT p.* FROM policies p WHERE NOT EXISTS(SELECT 1 FROM unnest($1::text[])term WHERE strpos(lower(concat_ws(' ',p.name,p.target_path,p.backup_type,p.scheduler,p.schedule,p.storage,CASE WHEN p.enabled THEN 'Включен' ELSE 'Отключен' END)),lower(term))=0)
) `

func (s *Service) GetPolicyPage(query string, page int) (PolicyPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := PolicyPage{Items: []Policy{}, Paths: map[string]string{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	args, err := catalogMatchArgs(CatalogCriteria{Query: query})
	if err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, policyPageCTE+`SELECT count(*) FROM matched`, args[0]).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, policyPageCTE+`SELECT to_jsonb(p),target_path FROM matched p ORDER BY lower(name),id LIMIT 25 OFFSET $2`, args[0], out.Page*25)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var path string
		if err = rows.Scan(&raw, &path); err != nil {
			return out, err
		}
		p, e := decodePolicy(raw)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
		out.Paths[p.ID] = path
	}
	return out, rows.Err()
}
