package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/database"
)

type Object struct {
	Attributes  Attributes `json:"attributes"`
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	ParentID    string     `json:"parentId"`
	Name        string     `json:"name"`
	Address     string     `json:"address"`
	OS          string     `json:"os"`
	Environment string     `json:"environment"`
	Status      string     `json:"status"`
	Description string     `json:"description"`
	DBMS        string     `json:"dbms"`
	DBMSVersion string     `json:"dbmsVersion"`
	Port        *int       `json:"port"`
	Purpose     string     `json:"purpose"`
	Version     int64      `json:"version"`
}
type Catalog struct {
	Servers   []Object `json:"servers"`
	Instances []Object `json:"instances"`
	Databases []Object `json:"databases"`
}

func table(kind string) (string, error) {
	switch kind {
	case "server":
		return "servers", nil
	case "instance":
		return "instances", nil
	case "database":
		return "databases", nil
	}
	return "", errors.New("Неизвестный тип объекта")
}
func (s *Service) readTransaction(ctx context.Context, write, admin bool) (pgx.Tx, error) {
	if s.pool == nil {
		return nil, errors.New("Нет подключения")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, errors.New(database.PublicError(err))
	}
	if err = s.authorize(ctx, tx, write, admin); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func (s *Service) GetCatalog() (Catalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return Catalog{}, err
	}
	defer tx.Rollback(ctx)
	return loadCatalog(ctx, tx)
}
func loadCatalog(ctx context.Context, tx pgx.Tx) (Catalog, error) {
	result := Catalog{Servers: []Object{}, Instances: []Object{}, Databases: []Object{}}
	// One repeatable-read snapshot avoids returning orphaned tree nodes during concurrent edits.
	for _, kind := range []string{"server", "instance", "database"} {
		t, _ := table(kind)
		rows, err := tx.Query(ctx, "SELECT to_jsonb(t) FROM "+t+" t ORDER BY lower(name),id")
		if err != nil {
			return result, errors.New(database.PublicError(err))
		}
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				rows.Close()
				return result, errors.New(database.PublicError(err))
			}
			o, err := decodeObject(raw, kind)
			if err != nil {
				rows.Close()
				return result, err
			}
			switch kind {
			case "server":
				result.Servers = append(result.Servers, o)
			case "instance":
				result.Instances = append(result.Instances, o)
			case "database":
				result.Databases = append(result.Databases, o)
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return result, errors.New(database.PublicError(err))
		}
	}
	return result, nil
}
func decodeObject(raw []byte, kind string) (Object, error) {
	var o Object
	if err := json.Unmarshal(raw, &o); err != nil {
		return o, err
	}
	var extra struct {
		ServerID    string `json:"server_id"`
		InstanceID  string `json:"instance_id"`
		DBMSVersion string `json:"dbms_version"`
	}
	if err := json.Unmarshal(raw, &extra); err != nil {
		return o, err
	}
	if o.Attributes.Administration == nil {
		o.Attributes.Administration = []string{}
	}
	o.Kind = kind
	o.DBMSVersion = extra.DBMSVersion
	if kind == "instance" {
		o.ParentID = extra.ServerID
	}
	if kind == "database" {
		o.ParentID = extra.InstanceID
		if o.DBMS == "Oracle Database" {
			o.ParentID = extra.ServerID
		}
	}
	return o, nil
}
func (s *Service) SaveObject(o Object) (Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return o, err
	}
	defer tx.Rollback(ctx)
	result, err := s.saveObjectTx(ctx, tx, o)
	if err != nil {
		return o, err
	}
	if err = tx.Commit(ctx); err != nil {
		return o, errors.New(database.PublicError(err))
	}
	return result, nil
}
func (s *Service) saveObjectTx(ctx context.Context, tx pgx.Tx, o Object) (Object, error) {
	return s.saveObjectTxAction(ctx, tx, o, false)
}
func (s *Service) saveObjectTxAction(ctx context.Context, tx pgx.Tx, o Object, restore bool) (Object, error) {
	writer, err := s.catalogWriter(ctx, tx)
	if err != nil {
		return o, err
	}
	return writer.saveObject(ctx, o, versionWriteMode(restore))
}
