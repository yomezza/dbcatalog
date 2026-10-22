package application

import (
	"context"
	"errors"
	"time"

	"github.com/yomezza/test/internal/database"
)

func (s *Service) ListUsers() ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id::text,login,role,state FROM users ORDER BY lower(login)`)
	if err != nil {
		return nil, errors.New(database.PublicError(err))
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err = rows.Scan(&u.ID, &u.Login, &u.Role, &u.State); err != nil {
			return nil, errors.New(database.PublicError(err))
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s *Service) UpdateUser(id, role, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if role != "admin" && role != "editor" && role != "viewer" {
		return errors.New("Неизвестная роль")
	}
	if state != "pending" && state != "active" && state != "blocked" && state != "disabled" {
		return errors.New("Неизвестное состояние")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if id == s.user.ID {
		return errors.New("Нельзя изменять собственную роль и состояние")
	}
	// Serialize administrator changes; the acting administrator stays active.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(724194301114)`); err != nil {
		return errors.New(database.PublicError(err))
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET role=$2,state=$3 WHERE id=$1`, id, role, state)
	if err != nil {
		return errors.New(database.PublicError(err))
	}
	if tag.RowsAffected() != 1 {
		return errors.New("Пользователь не найден")
	}
	if err = event(ctx, tx, s.user.ID, "user_update", "user", id); err != nil {
		return errors.New(database.PublicError(err))
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New(database.PublicError(err))
	}
	return nil
}

type HistoryEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"createdAt"`
	Before    any       `json:"before"`
	After     any       `json:"after"`
}

func (s *Service) GetHistory(kind, id string) ([]HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := table(kind); err != nil && kind != "policy" {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT h.id,h.action,u.login,h.created_at,h.before_state,h.after_state FROM object_history h JOIN users u ON u.id=h.actor_id WHERE entity=$1 AND object_id=$2 ORDER BY h.id DESC LIMIT 100`, kind, id)
	if err != nil {
		return nil, errors.New(database.PublicError(err))
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		if err = rows.Scan(&e.ID, &e.Action, &e.Actor, &e.CreatedAt, &e.Before, &e.After); err != nil {
			return nil, errors.New(database.PublicError(err))
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
