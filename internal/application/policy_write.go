package application

import (
	"context"
	"errors"
	"github.com/yomezza/test/internal/security"
)

func (w catalogWriter) savePolicy(ctx context.Context, p Policy, mode writeMode) (Policy, error) {
	recreate := mode == writeRecreateDeleted
	restore := mode != writeNormal
	s, tx := w.service, w.tx
	var err error
	p, err = normalizePolicy(p)
	if err != nil {
		return p, err
	}
	var before any
	action := "create"
	creating := p.ID == "" || recreate
	if p.ID != "" && !recreate {
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
	} else if !recreate {
		p.ID = security.ID()
		p.Version = 0
	}
	if p.BackupType != "Не указано" {
		if err = w.choices.validate("backupType", p.BackupType, ""); err != nil {
			return p, err
		}
	}
	if restore {
		action = "restore"
	}
	var instanceID, databaseID any
	if p.Kind == "instance" {
		instanceID = p.ObjectID
	} else {
		databaseID = p.ObjectID
	}
	args := []any{p.ID, instanceID, databaseID, p.Name, p.Enabled, p.BackupType, p.Scheduler, p.Schedule, p.Storage, p.RetentionDays, p.Description, s.user.ID}
	if creating {
		args = append(args, p.Version+1)
		_, err = tx.Exec(ctx, `INSERT INTO backup_policies(id,instance_id,database_id,name,enabled,backup_type,scheduler,schedule,storage,retention_days,description,updated_by,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, args...)
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
	return p, nil
}
