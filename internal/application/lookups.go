package application

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/yomezza/test/internal/database"
)

// GetLookups combines managed dictionaries with DISTINCT names currently present
// in the selected schema. Removed values disappear unless still used elsewhere.
func (s *Service) GetLookups() (map[string][]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	result := map[string][]string{}
	rows, err := tx.Query(ctx, `SELECT category,value FROM lookup_values ORDER BY category,position,value`)
	if err != nil {
		return nil, errors.New(database.PublicError(err))
	}
	for rows.Next() {
		var k, v string
		if err = rows.Scan(&k, &v); err != nil {
			rows.Close()
			return nil, err
		}
		result[k] = append(result[k], v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, key := range []string{"location", "serviceOrganization", "supervisor", "responsible"} {
		rows, err := tx.Query(ctx, `SELECT DISTINCT btrim(attributes->>$1) FROM (SELECT attributes FROM servers UNION ALL SELECT attributes FROM instances UNION ALL SELECT attributes FROM databases) t WHERE length(btrim(attributes->>$1))>0`, key)
		if err != nil {
			return nil, errors.New(database.PublicError(err))
		}
		seen := map[string]bool{}
		for _, v := range result[key] {
			seen[v] = true
		}
		extra := []string{}
		for rows.Next() {
			var v string
			if err = rows.Scan(&v); err != nil {
				rows.Close()
				return nil, err
			}
			if !seen[v] {
				seen[v] = true
				extra = append(extra, v)
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		sort.Strings(extra)
		result[key] = append(result[key], extra...)
		if result[key] == nil {
			result[key] = []string{}
		}
	}
	return result, nil
}
