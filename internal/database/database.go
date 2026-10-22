package database

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/config"
)

func PoolConfig(c config.Connection, password string) (*pgxpool.Config, error) {
	c = c.Normalize()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	// URL encoding prevents passwords, usernames and database names becoming DSN options.
	u := url.URL{Scheme: "postgres", Host: c.Address(), Path: "/" + c.Database, User: url.UserPassword(c.Username, password)}
	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	if c.SSLRootCert != "" {
		q.Set("sslrootcert", c.SSLRootCert)
	}
	u.RawQuery = q.Encode()
	p, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, errors.New("Некорректные параметры TLS или подключения")
	}
	p.MaxConns = 8
	p.MinConns = 0
	p.MaxConnLifetime = 30 * time.Minute
	p.ConnConfig.ConnectTimeout = 10 * time.Second
	p.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{c.Schema}.Sanitize() + ", pg_catalog, pg_temp"
	p.ConnConfig.RuntimeParams["application_name"] = "CatalogDB"
	return p, nil
}
func Open(ctx context.Context, c config.Connection, password string) (*pgxpool.Pool, error) {
	p, err := PoolConfig(c, password)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, p)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// PublicError never exposes server messages, DSNs, passwords or user input.
func PublicError(err error) string {
	var state *MigrationStateError
	if errors.As(err, &state) {
		if state.Reason == "newer schema" {
			return "Схема создана более новой версией CatalogDB. Обновите приложение; откат схемы не выполняется."
		}
		return "Контрольная сумма применённой миграции изменена. Восстановите исходные файлы миграций; автоматическое изменение существующей истории запрещено."
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "Сервер не ответил вовремя. Проверьте адрес и доступность PostgreSQL."
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "CD001":
			return "Найдены одинаковые SID Oracle в разных экземплярах одного сервера. Обновление отменено без изменения данных; устраните неоднозначность до перехода на новую структуру"
		case "28P01", "28000":
			return "PostgreSQL отклонил имя пользователя или пароль."
		case "42501":
			return "Недостаточно прав PostgreSQL. Для первичной настройки нужны права создания схемы и таблиц."
		case "3D000":
			return "Указанная база данных не существует. Создайте её на PostgreSQL."
		case "23505":
			return "Запись с таким уникальным значением уже существует."
		case "23503":
			return "Операция нарушает связи объектов. Сначала обработайте дочерние записи."
		}
	}
	return "Операция с PostgreSQL не выполнена. Проверьте соединение, TLS, права доступа и версию схемы."
}
