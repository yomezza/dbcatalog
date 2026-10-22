package application

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
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
	return loadLookups(ctx, tx)
}
func loadLookups(ctx context.Context, tx pgx.Tx) (map[string][]string, error) {
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

	keys := []string{"location", "serviceOrganization", "supervisor", "responsible"}
	seen := map[string]map[string]bool{}
	extra := map[string][]string{}
	for _, key := range keys {
		seen[key] = map[string]bool{}
		for _, v := range result[key] {
			seen[key][v] = true
		}
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT category,value FROM(
 SELECT attributes FROM servers UNION ALL SELECT attributes FROM instances UNION ALL SELECT attributes FROM databases
 ) t CROSS JOIN LATERAL(VALUES
 ('location',btrim(attributes->>'location')),('serviceOrganization',btrim(attributes->>'serviceOrganization')),
 ('supervisor',btrim(attributes->>'supervisor')),('responsible',btrim(attributes->>'responsible'))
 ) v(category,value) WHERE length(value)>0`)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return nil, importDatabaseError(err)
		}
		if !seen[key][value] {
			seen[key][value] = true
			extra[key] = append(extra[key], value)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, importDatabaseError(err)
	}
	for _, key := range keys {
		sort.Strings(extra[key])
		result[key] = append(result[key], extra[key]...)
		if result[key] == nil {
			result[key] = []string{}
		}
	}
	return result, nil
}
