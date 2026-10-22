package application

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type choiceSet map[string]map[string]struct{}

// A snapshot belongs to one transaction/operation, never to a connection or user session.
func loadChoices(ctx context.Context, tx pgx.Tx) (choiceSet, error) {
	rows, err := tx.Query(ctx, `SELECT category,value FROM lookup_values`)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	defer rows.Close()
	out := choiceSet{}
	for rows.Next() {
		var category, value string
		if err = rows.Scan(&category, &value); err != nil {
			return nil, importDatabaseError(err)
		}
		if out[category] == nil {
			out[category] = map[string]struct{}{}
		}
		out[category][value] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return nil, importDatabaseError(err)
	}
	return out, nil
}
func (choices choiceSet) validate(category, value, previous string) error {
	if value == "" || value == previous {
		return nil
	}
	_, found := choices[category][value]
	if !found {
		label := map[string]string{"status": "Состояние", "environment": "Тип эксплуатации", "dbms": "Тип СУБД", "domain": "Доменное членство", "connectionMethod": "Подключение к серверу", "administration": "Работа АБД", "backupType": "Тип РК"}[category]
		return errors.New("Выберите значение из справочника «" + label + "»")
	}
	return nil
}
func (choices choiceSet) validateObject(o, old Object) error {
	for _, v := range []struct{ category, value, previous string }{{"status", o.Status, old.Status}, {"environment", o.Environment, old.Environment}, {"domain", o.Attributes.Domain, old.Attributes.Domain}, {"connectionMethod", o.Attributes.ConnectionMethod, old.Attributes.ConnectionMethod}} {
		if err := choices.validate(v.category, v.value, v.previous); err != nil {
			return err
		}
	}
	if o.Kind == "instance" {
		if err := choices.validate("dbms", o.DBMS, old.DBMS); err != nil {
			return err
		}
	}
	for _, v := range o.Attributes.Administration {
		if err := choices.validate("administration", v, ""); err != nil {
			return err
		}
	}
	return nil
}
