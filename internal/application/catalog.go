package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/database"
	"github.com/yomezza/test/internal/security"
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
	t, err := table(o.Kind)
	if err != nil {
		return o, err
	}
	o.Name = strings.TrimSpace(o.Name)
	o.Attributes, err = normalizeAttributes(o.Attributes)
	if err != nil {
		return o, err
	}
	if o.Name == "" || len([]rune(o.Name)) > 200 {
		return o, errors.New("Укажите название длиной до 200 символов")
	}
	if len(o.Description) > 20000 {
		return o, errors.New("Описание слишком длинное")
	}
	if o.Kind != "server" && o.ParentID == "" {
		return o, errors.New("Выберите родительский объект")
	}
	if o.Kind == "instance" && strings.TrimSpace(o.DBMS) == "" {
		return o, errors.New("Укажите тип СУБД")
	}
	if o.Port != nil && (*o.Port < 1 || *o.Port > 65535) {
		return o, errors.New("Порт должен быть от 1 до 65535")
	}
	var before any
	var previous Object
	action := "create"
	if o.ID != "" {
		action = "update"
		var raw []byte
		if err = tx.QueryRow(ctx, "SELECT to_jsonb(t) FROM "+t+" t WHERE id=$1 FOR UPDATE", o.ID).Scan(&raw); err != nil {
			return o, errors.New("Объект не найден")
		}
		old, e := decodeObject(raw, o.Kind)
		if e != nil {
			return o, e
		}
		if old.Version != o.Version {
			return o, errors.New("Объект уже изменён другим пользователем. Обновите каталог и повторите редактирование")
		}
		before = old
		previous = old
	} else {
		o.ID = security.ID()
		o.Version = 0
	}
	if o.Status == "" {
		o.Status = "Эксплуатируется"
	}
	if err = validateObjectChoices(ctx, tx, o, previous); err != nil {
		return o, err
	}
	var sql string
	var args []any
	switch o.Kind {
	case "server":
		args = []any{o.ID, o.Name, o.Address, o.OS, o.Environment, o.Status, o.Description, s.user.ID}
		if action == "create" {
			sql = `INSERT INTO servers(id,name,address,os,environment,status,description,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`
		} else {
			sql = `UPDATE servers SET name=$2,address=$3,os=$4,environment=$5,status=$6,description=$7,updated_by=$8,version=version+1,updated_at=now() WHERE id=$1`
		}
	case "instance":
		args = []any{o.ID, o.ParentID, o.Name, o.DBMS, o.DBMSVersion, o.Port, o.Status, o.Description, s.user.ID}
		if action == "create" {
			sql = `INSERT INTO instances(id,server_id,name,dbms,dbms_version,port,status,description,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`
		} else {
			sql = `UPDATE instances SET server_id=$2,name=$3,dbms=$4,dbms_version=$5,port=$6,status=$7,description=$8,updated_by=$9,version=version+1,updated_at=now() WHERE id=$1`
		}
	case "database":
		args = []any{o.ID, o.ParentID, o.Name, o.Purpose, o.Status, o.Description, s.user.ID}
		if action == "create" {
			sql = `INSERT INTO databases(id,instance_id,name,purpose,status,description,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7)`
		} else {
			sql = `UPDATE databases SET instance_id=$2,name=$3,purpose=$4,status=$5,description=$6,updated_by=$7,version=version+1,updated_at=now() WHERE id=$1`
		}
	}
	if _, err = tx.Exec(ctx, sql, args...); err != nil {
		return o, errors.New(database.PublicError(err))
	}
	attributes, err := json.Marshal(o.Attributes)
	if err != nil {
		return o, err
	}
	if _, err = tx.Exec(ctx, "UPDATE "+t+" SET attributes=$2,environment=$3 WHERE id=$1", o.ID, attributes, o.Environment); err != nil {
		return o, errors.New(database.PublicError(err))
	}
	o.Version++
	if err = history(ctx, tx, s.user.ID, action, o.Kind, o.ID, before, o); err != nil {
		return o, errors.New(database.PublicError(err))
	}

	return o, nil
}
