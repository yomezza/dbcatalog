package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/security"
)

type Policy struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	ObjectID      string `json:"objectId"`
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	BackupType    string `json:"backupType"`
	Scheduler     string `json:"scheduler"`
	Schedule      string `json:"schedule"`
	Storage       string `json:"storage"`
	RetentionDays int    `json:"retentionDays"`
	Description   string `json:"description"`
	Version       int64  `json:"version"`
}
type EffectivePolicy struct {
	Policy    *Policy `json:"policy"`
	Inherited bool    `json:"inherited"`
}

func decodePolicy(raw []byte) (Policy, error) {
	var p Policy
	err := json.Unmarshal(raw, &p)
	if err != nil {
		return p, err
	}
	var extra struct {
		InstanceID    string `json:"instance_id"`
		DatabaseID    string `json:"database_id"`
		BackupType    string `json:"backup_type"`
		RetentionDays int    `json:"retention_days"`
	}
	if err = json.Unmarshal(raw, &extra); err != nil {
		return p, err
	}
	p.Kind = "instance"
	p.ObjectID = extra.InstanceID
	if extra.DatabaseID != "" {
		p.Kind = "database"
		p.ObjectID = extra.DatabaseID
	}
	p.BackupType = extra.BackupType
	p.RetentionDays = extra.RetentionDays
	return p, nil
}
func (s *Service) ListPolicies() ([]Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT to_jsonb(p) FROM backup_policies p ORDER BY lower(name),id`)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	defer rows.Close()
	out := []Policy{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		p, err := decodePolicy(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) SavePolicy(p Policy) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.Kind != "instance" && p.Kind != "database" {
		return p, errors.New("Политика назначается экземпляру или базе данных")
	}
	p.Name = strings.TrimSpace(p.Name)
	p.BackupType = strings.TrimSpace(p.BackupType)
	p.Scheduler = strings.TrimSpace(p.Scheduler)
	p.Schedule = strings.TrimSpace(p.Schedule)
	p.Storage = strings.TrimSpace(p.Storage)
	if p.Name == "" || len([]rune(p.Name)) > 200 {
		return p, errors.New("Название политики: от 1 до 200 символов")
	}
	if p.ObjectID == "" {
		return p, errors.New("Выберите объект назначения")
	}
	if p.BackupType == "" {
		p.BackupType = "Не указано"
	}
	if p.Scheduler == "" {
		p.Scheduler = "Не указано"
	}
	if p.RetentionDays < 0 || p.RetentionDays > 36500 {
		return p, errors.New("Срок хранения: от 0 до 36500 дней")
	}
	for _, v := range []string{p.BackupType, p.Scheduler, p.Schedule, p.Storage, p.Description} {
		if len(v) > 20000 {
			return p, errors.New("Параметр политики слишком длинный")
		}
	}
	if p.Enabled && (p.BackupType == "Не указано" || p.Scheduler == "Не указано" || strings.TrimSpace(p.Schedule) == "" || strings.TrimSpace(p.Storage) == "" || p.RetentionDays == 0) {
		return p, errors.New("Для включённой политики укажите тип РК, планировщик, расписание, путь и срок хранения")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	var before any
	action := "create"
	if p.ID != "" {
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM backup_policies p WHERE id=$1 FOR UPDATE`, p.ID).Scan(&raw); err != nil {
			return p, errors.New("Политика не найдена")
		}
		old, e := decodePolicy(raw)
		if e != nil {
			return p, e
		}
		if old.Version != p.Version {
			return p, errors.New("Политика уже изменена другим пользователем. Обновите список")
		}
		before = old
		action = "update"
	} else {
		p.ID = security.ID()
		p.Version = 0
	}
	if p.BackupType != "Не указано" {
		if err = validateChoice(ctx, tx, "backupType", p.BackupType, ""); err != nil {
			return p, err
		}
	}
	var instanceID, databaseID any
	if p.Kind == "instance" {
		instanceID = p.ObjectID
	} else {
		databaseID = p.ObjectID
	}
	args := []any{p.ID, instanceID, databaseID, p.Name, p.Enabled, p.BackupType, p.Scheduler, p.Schedule, p.Storage, p.RetentionDays, p.Description, s.user.ID}
	if action == "create" {
		_, err = tx.Exec(ctx, `INSERT INTO backup_policies(id,instance_id,database_id,name,enabled,backup_type,scheduler,schedule,storage,retention_days,description,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, args...)
	} else {
		_, err = tx.Exec(ctx, `UPDATE backup_policies SET instance_id=$2,database_id=$3,name=$4,enabled=$5,backup_type=$6,scheduler=$7,schedule=$8,storage=$9,retention_days=$10,description=$11,updated_by=$12,version=version+1,updated_at=now() WHERE id=$1`, args...)
	}
	if err != nil {
		return p, importDatabaseError(err)
	}
	p.Version++
	if err = history(ctx, tx, s.user.ID, action, "policy", p.ID, before, p); err != nil {
		return p, importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return p, importDatabaseError(err)
	}
	return p, nil
}
func (s *Service) DeletePolicy(id string, version int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM backup_policies p WHERE id=$1 FOR UPDATE`, id).Scan(&raw); err != nil {
		return errors.New("Политика не найдена")
	}
	old, err := decodePolicy(raw)
	if err != nil {
		return err
	}
	if old.Version != version {
		return errors.New("Политика изменена другим пользователем. Обновите список")
	}
	if _, err = tx.Exec(ctx, `DELETE FROM backup_policies WHERE id=$1`, id); err != nil {
		return importDatabaseError(err)
	}
	if err = history(ctx, tx, s.user.ID, "delete", "policy", id, old, nil); err != nil {
		return importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return importDatabaseError(err)
	}
	return nil
}
func (s *Service) GetEffectivePolicy(databaseID string) (EffectivePolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return EffectivePolicy{}, err
	}
	defer tx.Rollback(ctx)
	var instanceID string
	if err = tx.QueryRow(ctx, `SELECT instance_id::text FROM databases WHERE id=$1`, databaseID).Scan(&instanceID); err != nil {
		return EffectivePolicy{}, errors.New("База данных не найдена")
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM backup_policies p WHERE database_id=$1 OR instance_id=$2 ORDER BY (database_id IS NOT NULL) DESC LIMIT 1`, databaseID, instanceID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return EffectivePolicy{}, nil
	}
	if err != nil {
		return EffectivePolicy{}, importDatabaseError(err)
	}
	p, err := decodePolicy(raw)
	if err != nil {
		return EffectivePolicy{}, err
	}
	return EffectivePolicy{Policy: &p, Inherited: p.Kind == "instance"}, nil
}
