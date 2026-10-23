package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Movement struct {
	ID         int64     `json:"id"`
	FromPath   string    `json:"fromPath"`
	ToPath     string    `json:"toPath"`
	Reason     string    `json:"reason"`
	Version    int64     `json:"version"`
	Actor      string    `json:"actor"`
	CreatedAt  time.Time `json:"createdAt"`
	FromPolicy *Policy   `json:"fromPolicy"`
	ToPolicy   *Policy   `json:"toPolicy"`
}
type MovementPage struct {
	Items []Movement `json:"items"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
}
type MoveRequest struct {
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Version       int64  `json:"version"`
	ParentID      string `json:"parentId"`
	ParentVersion int64  `json:"parentVersion"`
	Reason        string `json:"reason"`
}
type MovePreview struct {
	FromDBMS    string   `json:"fromDBMS"`
	ToDBMS      string   `json:"toDBMS"`
	Object      Object   `json:"object"`
	Destination Object   `json:"destination"`
	FromPath    string   `json:"fromPath"`
	ToPath      string   `json:"toPath"`
	FromPolicy  *Policy  `json:"fromPolicy"`
	ToPolicy    *Policy  `json:"toPolicy"`
	Databases   int      `json:"databases"`
	Attachments int      `json:"attachments"`
	Fingerprint string   `json:"fingerprint"`
	Conflicts   []string `json:"conflicts"`
}

func (s *Service) PreviewMove(r MoveRequest) (MovePreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out MovePreview
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	raw, err := json.Marshal(r)
	if err != nil {
		return out, err
	}
	var result []byte
	if err = tx.QueryRow(ctx, `SELECT api_move_preview($1)`, raw).Scan(&result); err != nil {
		return out, importDatabaseError(err)
	}
	err = json.Unmarshal(result, &out)
	return out, err
}
func (s *Service) ApplyMove(r MoveRequest, fingerprint string) (Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Object
	if fingerprint == "" {
		return out, errors.New("Проверьте предпросмотр перемещения")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if s.pool == nil {
		return out, errors.New("Нет подключения")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, importDatabaseError(err)
	}
	defer tx.Rollback(ctx)
	if err = s.authorize(ctx, tx, true, false); err != nil {
		return out, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return out, err
	}
	var result []byte
	if err = tx.QueryRow(ctx, `SELECT api_move_object($1,$2)`, raw, fingerprint).Scan(&result); err != nil {
		return out, importDatabaseError(err)
	}
	if err = json.Unmarshal(result, &out); err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, importDatabaseError(err)
	}
	return out, nil
}
func (s *Service) GetMovements(kind, id string, page int) (MovementPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := MovementPage{Items: []Movement{}}
	if kind != "instance" && kind != "database" {
		return out, errors.New("Нет перемещений для этого типа объекта")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM object_movements WHERE entity=$1 AND object_id=$2`, kind, id).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, `SELECT m.id,m.from_path,m.to_path,m.reason,m.object_version,u.login,m.created_at,m.from_policy,m.to_policy FROM object_movements m JOIN users u ON u.id=m.actor_id WHERE m.entity=$1 AND m.object_id=$2 ORDER BY m.id DESC LIMIT 25 OFFSET $3`, kind, id, out.Page*25)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Movement
		var fp, tp []byte
		if err = rows.Scan(&m.ID, &m.FromPath, &m.ToPath, &m.Reason, &m.Version, &m.Actor, &m.CreatedAt, &fp, &tp); err != nil {
			return out, err
		}
		if len(fp) > 0 {
			m.FromPolicy = &Policy{}
			if err = json.Unmarshal(fp, m.FromPolicy); err != nil {
				return out, err
			}
		}
		if len(tp) > 0 {
			m.ToPolicy = &Policy{}
			if err = json.Unmarshal(tp, m.ToPolicy); err != nil {
				return out, err
			}
		}
		out.Items = append(out.Items, m)
	}
	return out, rows.Err()
}
