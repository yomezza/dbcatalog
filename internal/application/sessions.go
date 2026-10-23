package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/yomezza/test/internal/security"
)

const sessionStore = "CatalogDB.Session"

type savedSession struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// LoginWithSession authenticates normally; only a random token is persisted in the OS store.
func (s *Service) LoginWithSession(login, password string, hours int) (User, error) {
	if hours < 0 || hours > 168 {
		return User{}, errors.New("Срок сохранения входа: от 1 до 168 часов, либо 0 без сохранения")
	}
	u, err := s.Login(login, password)
	if err != nil {
		return u, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if s.user == nil || s.user.ID != u.ID {
		return User{}, errors.New("Сессия изменилась. Повторите вход")
	}
	s.clearSavedSessionLocked(ctx)
	if hours == 0 {
		return u, nil
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return User{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	id := security.ID()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var expires time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO user_sessions(id,user_id,token_hash,expires_at)VALUES($1,$2,$3,now()+make_interval(hours=>$4))RETURNING expires_at`, id, u.ID, hash, hours).Scan(&expires); err != nil {
		return User{}, importDatabaseError(err)
	}
	raw, _ := json.Marshal(savedSession{id, token})
	if err = s.store.Set(sessionStore, s.connectionKey, string(raw)); err != nil {
		s.user = nil
		return User{}, errors.New("Не удалось сохранить вход в системном хранилище. Повторите вход без сохранения")
	}
	if err = tx.Commit(ctx); err != nil {
		_ = s.store.Set(sessionStore, s.connectionKey, "")
		s.user = nil
		return User{}, importDatabaseError(err)
	}
	s.sessionID = id
	s.sessionExpires = expires
	return u, nil
}
func (s *Service) clearSavedSessionLocked(ctx context.Context) {
	if s.sessionID != "" && s.user != nil && s.pool != nil {
		_, _ = s.pool.Exec(ctx, `UPDATE user_sessions SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, s.sessionID, s.user.ID)
	}

	raw, err := s.store.Get(sessionStore, s.connectionKey)
	if err == nil {
		var ss savedSession
		if json.Unmarshal([]byte(raw), &ss) == nil && ss.ID != "" && s.pool != nil {
			// Match the secret as well as ID; a malformed local entry cannot revoke another session.
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(ss.Token)))
			_, _ = s.pool.Exec(ctx, `UPDATE user_sessions SET revoked_at=now() WHERE id=$1 AND token_hash=$2`, ss.ID, hash)
		}
	}
	_ = s.store.Set(sessionStore, s.connectionKey, "")
	s.sessionID = ""
	s.sessionExpires = time.Time{}
}
func (s *Service) ResumeSession() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool == nil {
		return Status{}, errors.New("Нет подключения")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := s.store.Get(sessionStore, s.connectionKey)
	if err != nil || raw == "" {
		return s.status(ctx)
	}
	var ss savedSession
	var id pgtype.UUID
	if json.Unmarshal([]byte(raw), &ss) != nil || id.Scan(ss.ID) != nil || !id.Valid || len(ss.Token) != 43 {
		_ = s.store.Set(sessionStore, s.connectionKey, "")
		return s.status(ctx)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(ss.Token)))
	var u User
	var expires time.Time
	err = s.pool.QueryRow(ctx, `SELECT u.id::text,u.login,u.role,u.state,x.expires_at FROM user_sessions x JOIN users u ON u.id=x.user_id WHERE x.id=$1 AND x.token_hash=$2 AND x.revoked_at IS NULL AND x.expires_at>now() AND u.state='active'`, ss.ID, hash).Scan(&u.ID, &u.Login, &u.Role, &u.State, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = s.store.Set(sessionStore, s.connectionKey, "")
		return s.status(ctx)
	}
	if err != nil {
		return Status{}, importDatabaseError(err)
	}
	s.user = &u
	s.sessionID = ss.ID
	s.sessionExpires = expires
	return s.status(ctx)
}
