package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/yomezza/test/internal/database"
	"github.com/yomezza/test/internal/security"
	"strings"
)

func (w catalogWriter) saveObject(ctx context.Context, o Object, mode writeMode) (Object, error) {
	recreate := mode == writeRecreateDeleted
	restore := mode != writeNormal
	s, tx := w.service, w.tx
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
	if o.Kind == "instance" && o.DBMS == "Oracle Database" {
		return o, errors.New("Oracle Database добавляется как база непосредственно на сервер, без экземпляра")
	}
	if o.Kind == "database" && o.DBMS != "" && o.DBMS != "Oracle Database" {
		return o, errors.New("Для обычной БД выберите экземпляр; прямое размещение на сервере доступно для Oracle Database")
	}
	if o.Kind == "database" && o.DBMS == "" {
		o.Port = nil
		o.DBMSVersion = ""
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
	creating := o.ID == "" || recreate
	if o.ID != "" && !recreate {
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
	} else if !recreate {
		o.ID = security.ID()
		o.Version = 0
	}
	if o.Status == "" {
		o.Status = "Эксплуатируется"
	}
	if err = w.choices.validateObject(o, previous); err != nil {
		return o, err
	}
	if restore {
		action = "restore"
	}
	attributes, err := json.Marshal(o.Attributes)
	if err != nil {
		return o, err
	}
	var sql string
	var args []any
	switch o.Kind {
	case "server":
		args = []any{o.ID, o.Name, o.Address, o.OS, o.Environment, o.Status, o.Description, s.user.ID, attributes, o.Version + 1}
		if creating {
			sql = `INSERT INTO servers(id,name,address,os,environment,status,description,updated_by,attributes,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`
		} else {
			sql = `UPDATE servers SET name=$2,address=$3,os=$4,environment=$5,status=$6,description=$7,updated_by=$8,attributes=$9,version=$10,updated_at=now() WHERE id=$1`
		}
	case "instance":
		args = []any{o.ID, o.ParentID, o.Name, o.DBMS, o.DBMSVersion, o.Port, o.Status, o.Description, s.user.ID, o.Environment, attributes, o.Version + 1}
		if creating {
			sql = `INSERT INTO instances(id,server_id,name,dbms,dbms_version,port,status,description,updated_by,environment,attributes,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`
		} else {
			sql = `UPDATE instances SET server_id=$2,name=$3,dbms=$4,dbms_version=$5,port=$6,status=$7,description=$8,updated_by=$9,environment=$10,attributes=$11,version=$12,updated_at=now() WHERE id=$1`
		}
	case "database":
		var instanceID, serverID any
		if o.DBMS == "Oracle Database" {
			serverID = o.ParentID
		} else {
			instanceID = o.ParentID
		}
		args = []any{o.ID, instanceID, o.Name, o.Purpose, o.Status, o.Description, s.user.ID, serverID, o.DBMS, o.DBMSVersion, o.Port, o.Environment, attributes, o.Version + 1}
		if creating {
			sql = `INSERT INTO databases(id,instance_id,name,purpose,status,description,updated_by,server_id,dbms,dbms_version,port,environment,attributes,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
		} else {
			sql = `UPDATE databases SET instance_id=$2,name=$3,purpose=$4,status=$5,description=$6,updated_by=$7,server_id=$8,dbms=$9,dbms_version=$10,port=$11,environment=$12,attributes=$13,version=$14,updated_at=now() WHERE id=$1`
		}

	}
	if _, err = tx.Exec(ctx, sql, args...); err != nil {
		return o, errors.New(database.PublicError(err))
	}
	o.Version++
	if err = history(ctx, tx, s.user.ID, action, o.Kind, o.ID, before, o); err != nil {
		return o, errors.New(database.PublicError(err))
	}

	return o, nil
}
