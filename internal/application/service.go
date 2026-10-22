package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/config"
	"github.com/yomezza/test/internal/database"
	"github.com/yomezza/test/internal/security"
	"github.com/zalando/go-keyring"
)

type CredentialStore interface {
	Get(string, string) (string, error)
	Set(string, string, string) error
}
type OSStore struct{}

func (OSStore) Get(s, k string) (string, error) { return keyring.Get(s, k) }
func (OSStore) Set(s, k, v string) error        { return keyring.Set(s, k, v) }

type User struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Role  string `json:"role"`
	State string `json:"state"`
}
type Status struct {
	Connected  bool   `json:"connected"`
	Schema     string `json:"schema"`
	NeedsAdmin bool   `json:"needsAdmin"`
	User       *User  `json:"user"`
}
type SavedConnection struct {
	Connection  config.Connection `json:"connection"`
	HasPassword bool              `json:"hasPassword"`
}
type Service struct {
	mu        sync.Mutex
	pool      *pgxpool.Pool
	schema    string
	user      *User
	path      string
	store     CredentialStore
	nextLogin time.Time
	dummyHash string
}

func New(path string, store CredentialStore) *Service {
	hash, _ := security.Hash(security.ID())
	return &Service{path: path, store: store, dummyHash: hash}
}
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
	s.user = nil
}
func (s *Service) GetConnection() (SavedConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := config.Load(s.path)
	if os.IsNotExist(err) {
		return SavedConnection{Connection: config.Connection{Host: "localhost", Port: 5432, Database: "catalogdb", Username: "catalogdb", Schema: config.DefaultSchema, SSLMode: "verify-full"}}, nil
	}
	if err != nil {
		return SavedConnection{}, errors.New("Не удалось прочитать настройки подключения")
	}
	_, err = s.store.Get("CatalogDB", c.CredentialKey())
	return SavedConnection{c, err == nil}, nil
}
func (s *Service) Connect(c config.Connection, password string, useSaved, remember bool) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c = c.Normalize()
	if err := c.Validate(); err != nil {
		return Status{}, err
	}
	if useSaved {
		var err error
		password, err = s.store.Get("CatalogDB", c.CredentialKey())
		if err != nil {
			return Status{}, errors.New("Сохранённый пароль не найден для этого подключения и схемы")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, c, password)
	if err != nil {
		return Status{}, errors.New(database.PublicError(err))
	}
	if err = database.Migrate(ctx, pool, c.Schema); err != nil {
		pool.Close()
		return Status{}, errors.New("Не удалось автоматически обновить выбранную схему. " + database.PublicError(err))
	}
	if remember {
		if err = s.store.Set("CatalogDB", c.CredentialKey(), password); err != nil {
			pool.Close()
			return Status{}, errors.New("Не удалось сохранить пароль в системном хранилище. Повторите подключение без сохранения пароля")
		}
	}
	if err = config.Save(s.path, c); err != nil {
		pool.Close()
		return Status{}, errors.New("Не удалось сохранить несекретные настройки подключения")
	}
	if s.pool != nil {
		s.pool.Close()
	}
	s.pool = pool
	s.schema = c.Schema
	s.user = nil
	return s.status(ctx)
}
func (s *Service) status(ctx context.Context) (Status, error) {
	result := Status{Connected: s.pool != nil, Schema: s.schema, User: s.user}
	if s.pool == nil {
		return result, nil
	}
	if err := s.pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM users)`).Scan(&result.NeedsAdmin); err != nil {
		return Status{}, errors.New(database.PublicError(err))
	}
	return result, nil
}
func (s *Service) GetStatus() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.status(ctx)
}
func validateLogin(login string) error {
	if utf8.RuneCountInString(login) < 3 || utf8.RuneCountInString(login) > 80 || strings.ContainsAny(login, "\r\n\t") {
		return errors.New("Логин должен содержать от 3 до 80 символов без перевода строки")
	}
	return nil
}
func (s *Service) Register(login, password string, bootstrap bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool == nil {
		return errors.New("Сначала подключитесь к PostgreSQL")
	}
	login = strings.TrimSpace(login)
	if err := validateLogin(login); err != nil {
		return err
	}
	hash, err := security.Hash(password)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.New(database.PublicError(err))
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(724194301113)`); err != nil {
		return errors.New(database.PublicError(err))
	}
	var empty bool
	if err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM users)`).Scan(&empty); err != nil {
		return errors.New(database.PublicError(err))
	}
	if bootstrap != empty {
		return errors.New("Первоначальный администратор уже создан либо ещё не настроен")
	}
	role, state := "viewer", "pending"
	if bootstrap {
		role, state = "admin", "active"
	}
	id := security.ID()
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,login,password_hash,role,state) VALUES($1,$2,$3,$4,$5)`, id, login, hash, role, state); err != nil {
		return errors.New(database.PublicError(err))
	}
	if err = event(ctx, tx, id, "register", "user", id); err != nil {
		return errors.New(database.PublicError(err))
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New(database.PublicError(err))
	}
	return nil
}
func (s *Service) Login(login, password string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool == nil {
		return User{}, errors.New("Сначала подключитесь к PostgreSQL")
	}
	if time.Now().Before(s.nextLogin) {
		return User{}, errors.New("Подождите перед следующей попыткой входа")
	}
	s.nextLogin = time.Now().Add(time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var u User
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT id::text,login,password_hash,role,state FROM users WHERE lower(login)=lower($1)`, strings.TrimSpace(login)).Scan(&u.ID, &u.Login, &hash, &u.Role, &u.State)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return User{}, errors.New(database.PublicError(err))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		hash = s.dummyHash
	}
	valid := security.Verify(hash, password)
	if err != nil || !valid || u.State != "active" {
		var actor any
		if u.ID != "" {
			actor = u.ID
		}
		if _, auditErr := s.pool.Exec(ctx, `INSERT INTO audit_events(actor_id,action) VALUES($1,'login_failed')`, actor); auditErr != nil {
			return User{}, errors.New(database.PublicError(auditErr))
		}
		return User{}, errors.New("Неверный логин или пароль, либо учётная запись не активна")
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO audit_events(actor_id,action) VALUES($1,'login')`, u.ID); err != nil {
		return User{}, errors.New(database.PublicError(err))
	}
	s.user = &u
	return u, nil
}
func (s *Service) Logout() { s.mu.Lock(); defer s.mu.Unlock(); s.user = nil }
func (s *Service) authorize(ctx context.Context, tx pgx.Tx, write, admin bool) error {
	if s.user == nil {
		return errors.New("Войдите в приложение")
	}
	var role, state string
	if err := tx.QueryRow(ctx, `SELECT role,state FROM users WHERE id=$1 FOR SHARE`, s.user.ID).Scan(&role, &state); err != nil {
		return errors.New("Сессия недействительна")
	}
	if state != "active" || (write && role == "viewer") || (admin && role != "admin") {
		return errors.New("Недостаточно прав")
	}
	return nil
}
func event(ctx context.Context, tx pgx.Tx, actor, action, entity, id string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id) VALUES($1,$2,$3,$4)`, actor, action, entity, id)
	return err
}
func history(ctx context.Context, tx pgx.Tx, actor, action, entity, id string, before, after any) error {
	var b, a []byte
	var err error
	if before != nil {
		b, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		a, err = json.Marshal(after)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO object_history(entity,object_id,action,actor_id,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6)`, entity, id, action, actor, b, a)
	if err != nil {
		return err
	}
	return event(ctx, tx, actor, action, entity, id)
}
