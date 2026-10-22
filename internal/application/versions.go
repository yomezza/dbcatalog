package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type RecordVersion struct {
	Version   int64     `json:"version"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"createdAt"`
}
type VersionPage struct {
	Items          []RecordVersion `json:"items"`
	Total          int64           `json:"total"`
	Page           int             `json:"page"`
	CurrentVersion int64           `json:"currentVersion"`
	Exists         bool            `json:"exists"`
}

func versionTable(kind string) (string, error) {
	if kind == "policy" {
		return "backup_policies", nil
	}
	return table(kind)
}
func boundedPage(page int, total int64, size int) int {
	if page < 0 || total == 0 {
		return 0
	}
	last := int((total - 1) / int64(size))
	if page > last {
		return last
	}
	return page
}
func (s *Service) GetVersions(kind, id string, page int) (VersionPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := VersionPage{Items: []RecordVersion{}}
	t, err := versionTable(kind)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, true)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, "SELECT version FROM "+t+" WHERE id=$1", id).Scan(&out.CurrentVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, importDatabaseError(err)
	}
	out.Exists = err == nil
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM record_versions WHERE entity=$1 AND object_id=$2`, kind, id).Scan(&out.Total); err != nil {
		return out, importDatabaseError(err)
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, `SELECT v.version,u.login,v.action,v.created_at FROM record_versions v JOIN users u ON u.id=v.actor_id WHERE entity=$1 AND object_id=$2 ORDER BY version DESC LIMIT 25 OFFSET $3`, kind, id, out.Page*25)
	if err != nil {
		return out, importDatabaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var v RecordVersion
		if err = rows.Scan(&v.Version, &v.Actor, &v.Action, &v.CreatedAt); err != nil {
			return out, importDatabaseError(err)
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
func (s *Service) GetRecordVersion(kind, id string, version int64) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := versionTable(kind); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var snapshot any
	if err = tx.QueryRow(ctx, `SELECT snapshot FROM record_versions WHERE entity=$1 AND object_id=$2 AND version=$3`, kind, id, version).Scan(&snapshot); err != nil {
		return nil, errors.New("Версия не найдена для этой записи")
	}
	return snapshot, nil
}
func (s *Service) RestoreVersion(kind, id string, currentVersion, targetVersion int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := versionTable(kind)
	if err != nil {
		return err
	}
	if currentVersion < 1 || targetVersion < 1 || currentVersion == targetVersion {
		return errors.New("Выберите другую сохранённую версию")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var actual int64
	if err = tx.QueryRow(ctx, "SELECT version FROM "+t+" WHERE id=$1 FOR UPDATE", id).Scan(&actual); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("Запись удалена или недоступна. Возврат версии существующей записи невозможен")
		}
		return importDatabaseError(err)
	}
	if actual != currentVersion {
		return errors.New("Запись изменилась после открытия версий. Обновите список и проверьте изменения заново")
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT snapshot FROM record_versions WHERE entity=$1 AND object_id=$2 AND version=$3`, kind, id, targetVersion).Scan(&raw); err != nil {
		return errors.New("Версия не найдена для этой записи")
	}
	var resultVersion int64
	if kind == "policy" {
		var p Policy
		if err = json.Unmarshal(raw, &p); err != nil || p.ID != id {
			return errors.New("Снимок политики повреждён")
		}
		parent, err := table(p.Kind)
		if err != nil || p.Kind == "server" {
			return errors.New("Некорректный объект назначения исторической политики")
		}
		if err = historicalParentExists(ctx, tx, parent, p.ObjectID); err != nil {
			return err
		}
		p.Version = currentVersion
		saved, err := s.savePolicyTx(ctx, tx, p, true)
		if err != nil {
			return err
		}
		resultVersion = saved.Version
	} else {
		var o Object
		if err = json.Unmarshal(raw, &o); err != nil || o.ID != id || o.Kind != kind {
			return errors.New("Снимок записи повреждён")
		}
		if kind != "server" {
			parent := "instances"
			if kind == "instance" || o.DBMS == "Oracle Database" {
				parent = "servers"
			}
			if err = historicalParentExists(ctx, tx, parent, o.ParentID); err != nil {
				return err
			}
		}
		o.Version = currentVersion
		saved, err := s.saveObjectTxAction(ctx, tx, o, true)
		if err != nil {
			return err
		}
		resultVersion = saved.Version
	}
	details, _ := json.Marshal(map[string]int64{"sourceVersion": targetVersion, "previousVersion": currentVersion, "resultVersion": resultVersion})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id,details) VALUES($1,'version_restore',$2,$3,$4)`, s.user.ID, kind, id, details); err != nil {
		return importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return importDatabaseError(err)
	}
	return nil
}
func historicalParentExists(ctx context.Context, tx pgx.Tx, parent, id string) error {
	var found bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM "+parent+" WHERE id=$1)", nullableID(id)).Scan(&found); err != nil {
		return importDatabaseError(err)
	}
	if !found {
		return errors.New("Родительский объект исторической версии отсутствует. Возврат отменён")
	}
	return nil
}
