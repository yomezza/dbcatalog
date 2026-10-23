package application

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
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
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT api_lookups()`).Scan(&raw); err != nil {
		return nil, importDatabaseError(err)
	}
	result := map[string][]string{}
	err := json.Unmarshal(raw, &result)
	return result, err
}
