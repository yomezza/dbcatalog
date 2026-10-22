package application

import (
	"context"
	"time"
)

type HistoryPage struct {
	Items []HistoryEntry `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
}

func (s *Service) GetHistoryPage(kind, id string, page int) (HistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := HistoryPage{Items: []HistoryEntry{}}
	if _, err := versionTable(kind); err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM object_history WHERE entity=$1 AND object_id=$2`, kind, id).Scan(&out.Total); err != nil {
		return out, importDatabaseError(err)
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, `SELECT h.id,h.action,u.login,h.created_at,h.before_state,h.after_state FROM object_history h JOIN users u ON u.id=h.actor_id WHERE entity=$1 AND object_id=$2 ORDER BY h.id DESC LIMIT 25 OFFSET $3`, kind, id, out.Page*25)
	if err != nil {
		return out, importDatabaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var h HistoryEntry
		if err = rows.Scan(&h.ID, &h.Action, &h.Actor, &h.CreatedAt, &h.Before, &h.After); err != nil {
			return out, importDatabaseError(err)
		}
		out.Items = append(out.Items, h)
	}
	return out, rows.Err()
}
