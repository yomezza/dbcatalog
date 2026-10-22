package application

import (
	"context"
	"errors"
	"time"

	"github.com/yomezza/test/internal/database"
)

func (s *Service) ListFavorites() ([]Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	out := []Object{}
	for _, kind := range []string{"server", "instance", "database"} {
		t, _ := table(kind)
		rows, err := tx.Query(ctx, "SELECT to_jsonb(t) FROM "+t+" t JOIN favorites f ON f."+kind+"_id=t.id WHERE f.user_id=$1 ORDER BY lower(t.name),t.id", s.user.ID)
		if err != nil {
			return nil, errors.New(database.PublicError(err))
		}
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				rows.Close()
				return nil, err
			}
			o, err := decodeObject(raw, kind)
			if err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, o)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (s *Service) SetFavorite(kind, id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := table(kind)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	action := "favorite_remove"
	var changed int64
	if enabled {
		var exists string
		if err = tx.QueryRow(ctx, "SELECT id::text FROM "+t+" WHERE id=$1 FOR KEY SHARE", id).Scan(&exists); err != nil {
			return errors.New("Объект не найден")
		}
		tag, e := tx.Exec(ctx, "INSERT INTO favorites(user_id,"+kind+"_id) VALUES($1,$2) ON CONFLICT DO NOTHING", s.user.ID, id)
		err = e
		changed = tag.RowsAffected()
		action = "favorite_add"
	} else {
		tag, e := tx.Exec(ctx, "DELETE FROM favorites WHERE user_id=$1 AND "+kind+"_id=$2", s.user.ID, id)
		err = e
		changed = tag.RowsAffected()
	}
	if err != nil {
		return errors.New(database.PublicError(err))
	}
	if changed > 0 {
		if err = event(ctx, tx, s.user.ID, action, kind, id); err != nil {
			return errors.New(database.PublicError(err))
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New(database.PublicError(err))
	}
	return nil
}
