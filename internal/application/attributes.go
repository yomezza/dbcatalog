package application

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/database"
)

type Attributes struct {
	InformationSystem   string   `json:"informationSystem"`
	ConnectionMethod    string   `json:"connectionMethod"`
	Domain              string   `json:"domain"`
	Location            string   `json:"location"`
	ServiceOrganization string   `json:"serviceOrganization"`
	Supervisor          string   `json:"supervisor"`
	Responsible         string   `json:"responsible"`
	OSVersion           string   `json:"osVersion"`
	OSFamily            string   `json:"osFamily"`
	Administration      []string `json:"administration"`
	PersonalData        string   `json:"personalData"`
}

func normalizeAttributes(a Attributes) (Attributes, error) {
	for _, v := range []*string{&a.InformationSystem, &a.ConnectionMethod, &a.Domain, &a.Location, &a.ServiceOrganization, &a.Supervisor, &a.Responsible, &a.OSVersion, &a.OSFamily, &a.PersonalData} {
		*v = strings.TrimSpace(*v)
		if utf8.RuneCountInString(*v) > 1000 {
			return a, errors.New("Значение дополнительного поля слишком длинное")
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, v := range a.Administration {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) > 30 {
		return a, errors.New("Слишком много значений «Работа АБД»")
	}
	a.Administration = out
	return a, nil
}
func validateChoice(ctx context.Context, tx pgx.Tx, category, value, previous string) error {
	if value == "" || value == previous {
		return nil
	}
	var found bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM lookup_values WHERE category=$1 AND value=$2)`, category, value).Scan(&found); err != nil {
		return errors.New(database.PublicError(err))
	}
	if !found {
		label := map[string]string{"status": "Состояние", "environment": "Тип эксплуатации", "dbms": "Тип СУБД", "domain": "Доменное членство", "connectionMethod": "Подключение к серверу", "administration": "Работа АБД", "backupType": "Тип РК"}[category]
		return errors.New("Выберите значение из справочника «" + label + "»")
	}
	return nil
}
func validateObjectChoices(ctx context.Context, tx pgx.Tx, o, old Object) error {
	for _, v := range []struct{ category, value, previous string }{{"status", o.Status, old.Status}, {"environment", o.Environment, old.Environment}, {"domain", o.Attributes.Domain, old.Attributes.Domain}, {"connectionMethod", o.Attributes.ConnectionMethod, old.Attributes.ConnectionMethod}} {
		if err := validateChoice(ctx, tx, v.category, v.value, v.previous); err != nil {
			return err
		}
	}
	if o.Kind == "instance" {
		if err := validateChoice(ctx, tx, "dbms", o.DBMS, old.DBMS); err != nil {
			return err
		}
	}
	for _, v := range o.Attributes.Administration {
		if err := validateChoice(ctx, tx, "administration", v, ""); err != nil {
			return err
		}
	}
	return nil
}
