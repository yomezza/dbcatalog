package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/database"
	"github.com/yomezza/test/internal/spreadsheet"
)

type importItem struct {
	server, instance, database Object
	number                     int
	errors                     []string
}
type objectStamp struct {
	ID      string
	Version int64
}
type importPlan struct {
	policyConfig *PolicyImportConfig
	policyStamp  string
	policies     []plannedImportPolicy
	excluded     map[int]bool
	workbook     spreadsheet.Workbook
	preview      ImportPreview
	items        []importItem
	versions     map[string]objectStamp
	expires      time.Time
	userID       string
}
type objectIndex struct {
	servers, instances, databases map[string]Object
	ambiguous                     map[string]bool
}

func serverKey(name string) string             { return "s:" + strings.ToLower(strings.TrimSpace(name)) }
func instanceKey(server, name string) string   { return "i:" + server + "\x00" + name }
func databaseKey(instance, name string) string { return "d:" + instance + "\x00" + name }
func oracleKey(server, name string) string     { return databaseKey("oracle:"+server, name) }
func (item importItem) isOracle() bool         { return item.database.DBMS == "Oracle Database" }
func (item importItem) keys() (string, string, string) {
	sk := serverKey(item.server.Name)
	ik := instanceKey(sk, item.instance.Name)
	dk := databaseKey(ik, item.database.Name)
	if item.isOracle() {
		ik = ""
		dk = oracleKey(sk, item.database.Name)
	}
	return sk, ik, dk
}
func loadIndex(ctx context.Context, tx pgx.Tx) (objectIndex, error) {
	idx := objectIndex{map[string]Object{}, map[string]Object{}, map[string]Object{}, map[string]bool{}}
	serverIDs, instanceIDs := map[string]string{}, map[string]string{}
	for _, kind := range []string{"server", "instance", "database"} {
		t, _ := table(kind)
		rows, err := tx.Query(ctx, "SELECT (to_jsonb(t)-'search_text') FROM "+t+" t ORDER BY id")
		if err != nil {
			return idx, err
		}
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				rows.Close()
				return idx, err
			}
			o, e := decodeObject(raw, kind)
			if e != nil {
				rows.Close()
				return idx, e
			}
			switch kind {
			case "server":
				k := serverKey(o.Name)
				if _, ok := idx.servers[k]; ok {
					idx.ambiguous[k] = true
				}
				idx.servers[k] = o
				serverIDs[o.ID] = k
			case "instance":
				k := instanceKey(serverIDs[o.ParentID], o.Name)
				idx.instances[k] = o
				instanceIDs[o.ID] = k
			case "database":
				key := databaseKey(instanceIDs[o.ParentID], o.Name)
				if o.DBMS == "Oracle Database" {
					key = oracleKey(serverIDs[o.ParentID], o.Name)
				}
				if _, ok := idx.databases[key]; ok {
					idx.ambiguous[key] = true
				}
				idx.databases[key] = o
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return idx, err
		}
	}
	return idx, nil
}
func (idx objectIndex) get(key string) Object {
	if strings.HasPrefix(key, "s:") {
		return idx.servers[key]
	}
	if strings.HasPrefix(key, "i:") {
		return idx.instances[key]
	}
	return idx.databases[key]
}
func splitAdministration(s string) []string {
	values := strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' || r == '\r' || r == '|' || r == ',' })
	a := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			a = append(a, v)
		}
	}
	return a
}
func objectsFromRow(row spreadsheet.Row) importItem {
	v := row.Values
	item := importItem{number: row.Number, errors: []string{}, server: Object{Kind: "server", Name: v[4], Address: v[9], OS: v[15], Status: "Эксплуатируется", Attributes: Attributes{ConnectionMethod: v[7], Domain: v[8], Location: v[10], OSVersion: v[16], OSFamily: v[17]}}, instance: Object{Kind: "instance", Name: v[1], DBMS: v[5], DBMSVersion: v[6], Status: "Эксплуатируется"}, database: Object{Kind: "database", Name: v[0], Status: v[2], Environment: v[3], Purpose: v[18], Attributes: Attributes{InformationSystem: v[11], ServiceOrganization: v[12], Supervisor: v[13], Responsible: v[14], Administration: splitAdministration(v[19]), PersonalData: v[20]}}}
	if v[5] == "Oracle Database" {
		item.database.DBMS = "Oracle Database"
		item.database.DBMSVersion = v[6]
	}
	return item
}
func validateImportItem(choices choiceSet, item *importItem) {

	for _, pair := range []struct{ name, value string }{{"Имя сервера", item.server.Name}, {"Экземпляр", item.instance.Name}, {"Тип СУБД", item.instance.DBMS}, {"SID", item.database.Name}} {
		if pair.name == "Экземпляр" && item.isOracle() {
			continue
		}
		if pair.value == "" {
			item.errors = append(item.errors, "Не заполнено: "+pair.name)
		}
		if len([]rune(pair.value)) > 200 {
			item.errors = append(item.errors, "Слишком длинное значение: "+pair.name)
		}
	}
	for _, o := range []*Object{&item.server, &item.instance, &item.database} {
		a, err := normalizeAttributes(o.Attributes)
		if err != nil {
			item.errors = append(item.errors, err.Error())
		} else {
			o.Attributes = a
		}
		if err := choices.validateObject(*o, Object{}); err != nil {
			item.errors = append(item.errors, err.Error())
		}
	}
}
func serverDifferences(a, b Object) []string {
	fields := []struct{ label, a, b string }{{"IPv4-адрес", a.Address, b.Address}, {"Операционная система", a.OS, b.OS}, {"Подключение к серверу", a.Attributes.ConnectionMethod, b.Attributes.ConnectionMethod}, {"Доменное членство", a.Attributes.Domain, b.Attributes.Domain}, {"Месторасположение", a.Attributes.Location, b.Attributes.Location}, {"Версия системы", a.Attributes.OSVersion, b.Attributes.OSVersion}, {"Семейство системы", a.Attributes.OSFamily, b.Attributes.OSFamily}}
	out := []string{}
	for _, f := range fields {
		if f.a != f.b {
			out = append(out, f.label)
		}
	}
	return out
}
func serverEquivalent(a, b Object) bool { return len(serverDifferences(a, b)) == 0 }
func sameObject(a, b Object) bool {
	a.ID = ""
	a.Version = 0
	b.ID = ""
	b.Version = 0
	return reflect.DeepEqual(a, b)
}
func mergeObject(old, incoming Object) Object {
	// Blank spreadsheet fields preserve existing values; clearing needs an explicit edit.
	out := incoming
	if old.Kind == "server" {
		out.Name = old.Name
	}
	if old.Kind != "database" {
		out.Status = old.Status
		out.Environment = old.Environment
	}
	out.ID = old.ID
	out.Version = old.Version
	for _, pair := range []struct {
		dst *string
		old string
	}{{&out.Address, old.Address}, {&out.OS, old.OS}, {&out.Environment, old.Environment}, {&out.Status, old.Status}, {&out.Description, old.Description}, {&out.DBMSVersion, old.DBMSVersion}, {&out.Purpose, old.Purpose}} {
		if *pair.dst == "" {
			*pair.dst = pair.old
		}
	}
	if out.Port == nil {
		out.Port = old.Port
	}
	out.Attributes = mergeAttributes(old.Attributes, incoming.Attributes)
	return out
}
func ensureUnchanged(idx objectIndex, versions map[string]objectStamp) error {
	for key, version := range versions {
		if idx.ambiguous[key] || (objectStamp{idx.get(key).ID, idx.get(key).Version}) != version {
			return errors.New("Каталог изменился после предпросмотра. Повторно выберите файл и проверьте результат")
		}
	}
	return nil
}
func (w catalogWriter) saveImportedObject(ctx context.Context, old, incoming Object, result *ImportResult) (Object, error) {
	if old.ID != "" {
		incoming = mergeObject(old, incoming)
		if sameObject(old, incoming) {
			return old, nil
		}
	}
	saved, err := w.saveObject(ctx, incoming, writeNormal)
	if err != nil {
		return saved, fmt.Errorf("Не удалось сохранить объект импорта: %w", err)
	}
	if old.ID != "" {
		result.ObjectsUpdated++
	} else {
		switch saved.Kind {
		case "server":
			result.ServersCreated++
		case "instance":
			result.InstancesCreated++
		case "database":
			result.DatabasesCreated++
		}
	}
	return saved, nil
}
func importDatabaseError(err error) error { return errors.New(database.PublicError(err)) }
