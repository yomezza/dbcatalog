package application

import (
	"context"
	"time"
)

type QualityIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Detail   string `json:"detail"`
}
type QualityPage struct {
	Items  []QualityIssue   `json:"items"`
	Counts map[string]int64 `json:"counts"`
	Total  int64            `json:"total"`
	Page   int              `json:"page"`
}

const qualityCTE = `WITH issues AS(SELECT * FROM api_quality_issues()) `

func (s *Service) GetQualityReport(code string, page int) (QualityPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := QualityPage{Items: []QualityIssue{}, Counts: map[string]int64{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	counts, err := tx.Query(ctx, qualityCTE+` SELECT code,count(*) FROM issues GROUP BY code`)
	if err != nil {
		return out, importDatabaseError(err)
	}
	for counts.Next() {
		var c string
		var n int64
		if err = counts.Scan(&c, &n); err != nil {
			counts.Close()
			return out, err
		}
		out.Counts[c] = n
		if code == "" || code == c {
			out.Total += n
		}
	}
	counts.Close()
	if err = counts.Err(); err != nil {
		return out, err
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, qualityCTE+` SELECT code,severity,kind,id::text,name,path,detail FROM issues WHERE $1='' OR code=$1 ORDER BY CASE severity WHEN 'danger' THEN 0 ELSE 1 END,code,lower(path),id LIMIT 25 OFFSET $2`, code, out.Page*25)
	if err != nil {
		return out, importDatabaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item QualityIssue
		if err = rows.Scan(&item.Code, &item.Severity, &item.Kind, &item.ID, &item.Name, &item.Path, &item.Detail); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
