package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

func (w catalogWriter) saveObject(ctx context.Context, o Object, mode writeMode) (Object, error) {
	_, err := table(o.Kind)
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

	raw, err := json.Marshal(o)
	if err != nil {
		return o, err
	}
	var result []byte
	if err = w.tx.QueryRow(ctx, `SELECT api_save_record($1,$2,$3,$4,$5)`, o.Kind, raw, o.Version, int(mode), w.moveReason).Scan(&result); err != nil {
		return o, importDatabaseError(err)
	}
	err = json.Unmarshal(result, &o)
	return o, err
}
