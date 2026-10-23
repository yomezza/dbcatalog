package application

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

type Distribution struct {
	Name      string `json:"name"`
	Instances int64  `json:"instances"`
	Databases int64  `json:"databases"`
}
type Statistics struct {
	Servers              int64          `json:"servers"`
	UniqueServers        int64          `json:"uniqueServers"`
	ServerAddresses      int64          `json:"serverAddresses"`
	Instances            int64          `json:"instances"`
	Databases            int64          `json:"databases"`
	Policies             int64          `json:"policies"`
	EnabledPolicies      int64          `json:"enabledPolicies"`
	ProtectedDatabases   int64          `json:"protectedDatabases"`
	UnprotectedDatabases int64          `json:"unprotectedDatabases"`
	DisabledDatabases    int64          `json:"disabledDatabases"`
	DBMS                 []Distribution `json:"dbms"`
	Statuses             []Distribution `json:"statuses"`
	Environments         []Distribution `json:"environments"`
	GeneratedAt          time.Time      `json:"generatedAt"`
}

func (s *Service) GetStatistics() (Statistics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return Statistics{}, err
	}
	defer tx.Rollback(ctx)
	return loadStatistics(ctx, tx)
}
func loadStatistics(ctx context.Context, tx pgx.Tx) (Statistics, error) {
	var out Statistics
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT api_statistics()`).Scan(&raw); err != nil {
		return out, importDatabaseError(err)
	}
	err := json.Unmarshal(raw, &out)
	return out, err
}
