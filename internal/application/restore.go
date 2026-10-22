package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// RestoreObject restores a recorded snapshot of an existing object as a new version.
// Historical snapshots are selected by ID on the server, never supplied by the client.
func (s *Service) RestoreObject(kind, id string, version, historyID int64, side string) (Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Object
	if _, err := table(kind); err != nil {
		return out, err
	}
	if side != "before" && side != "after" {
		return out, errors.New("Выберите состояние из истории")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, true)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT CASE WHEN $4='before' THEN before_state ELSE after_state END FROM object_history WHERE id=$1 AND entity=$2 AND object_id=$3`, historyID, kind, id, side).Scan(&raw); err != nil || len(raw) == 0 {
		return out, errors.New("Состояние не найдено в истории этого объекта")
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, errors.New("Не удалось прочитать состояние из истории")
	}
	if out.ID != id || out.Kind != kind {
		return out, errors.New("Историческое состояние не соответствует объекту")
	}
	if kind != "server" {
		parent := "instances"
		if kind == "instance" || out.DBMS == "Oracle Database" {
			parent = "servers"
		}
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM "+parent+" WHERE id=$1)", nullableID(out.ParentID)).Scan(&exists); err != nil {
			return out, importDatabaseError(err)
		}
		if !exists {
			return out, errors.New("Родительский объект исторического состояния отсутствует. Восстановление отменено")
		}
	}
	out.Version = version
	out, err = s.saveObjectTxAction(ctx, tx, out, true)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, importDatabaseError(err)
	}
	return out, nil
}
