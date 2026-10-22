package application

import (
	"bytes"
	"encoding/base64"
	"github.com/xuri/excelize/v2"
	"strings"
	"testing"
)

func oracleWorkbook(t *testing.T, rows [][21]string) string {
	t.Helper()
	wb := excelize.NewFile()
	defer wb.Close()
	headers := []string{"SID", "Экземпляр", "Состояние", "Тип эксплуатации", "Имя сервера", "Тип СУБД", "Версия СУБД", "Подключение к серверу", "Доменное членство", "IPv4-адрес", "Месторасположение", "Информационная система", "Обслуживающая организация", "Сопровождение (руководитель)", "Ответственное лицо", "Операционная система", "Версия системы", "Семейство системы", "Назначение", "Работа АБД", "Персональные данные"}
	for c, v := range headers {
		a, _ := excelize.CoordinatesToCellName(c+1, 1)
		wb.SetCellStr("Sheet1", a, v)
	}
	for r, row := range rows {
		for c, v := range row {
			a, _ := excelize.CoordinatesToCellName(c+1, r+2)
			wb.SetCellStr("Sheet1", a, v)
		}
	}
	buf, err := wb.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
func oracleRow(server, address, sid, instance, dbms string) [21]string {
	return [21]string{sid, instance, "Эксплуатируется", "Промышленный", server, dbms, "19c", "SSH", "отсутствует", address, "ЦОД", "ИС", "Организация", "Руководитель", "Ответственный", "Linux", "1", "Linux", "Назначение", "Мониторинг и контроль", "Нет"}
}
func TestOracleDirectImportCopiesAndRoundTrip(t *testing.T) {
	s, _, _ := catalogFixture(t)
	rows := [][21]string{oracleRow("host-a", "10.0.0.1", "COPY", "default", "Oracle Database"), oracleRow("host-b", "10.0.0.2", "COPY", "", "Oracle Database"), oracleRow("host-a", "10.0.0.1", "COPY", "pg", "PostgreSQL"), oracleRow("host-a", "10.0.0.1", "COPY", "pg-other", "PostgreSQL")}
	preview, err := s.PreviewExcel(oracleWorkbook(t, rows))
	if err != nil || preview.Valid != 4 {
		t.Fatal(preview, err)
	}
	result, err := s.ApplyExcel(preview.Token, false, false)
	if err != nil || result.InstancesCreated != 2 || result.DatabasesCreated != 4 || result.ServersCreated != 2 {
		t.Fatal(result, err)
	}
	c, err := s.GetCatalog()
	if err != nil {
		t.Fatal(err)
	}
	direct := []Object{}
	for _, d := range c.Databases {
		if d.DBMS == "Oracle Database" {
			direct = append(direct, d)
			if d.DBMSVersion != "19c" {
				t.Fatal(d)
			}
		}
	}
	if len(direct) != 2 || direct[0].ParentID == direct[1].ParentID {
		t.Fatal("same SID must be independent on different servers", direct)
	}
	for _, d := range direct {
		effective, e := s.GetEffectivePolicy(d.ID)
		if e != nil || effective.Policy != nil {
			t.Fatal(effective, e)
		}
		if _, e = s.SavePolicy(Policy{Kind: "database", ObjectID: d.ID, Name: "Oracle РК"}); e != nil {
			t.Fatal(e)
		}
		target, e := s.GetServerConnection(d.Kind, d.ID)
		if e != nil || target.Method != "SSH" {
			t.Fatal(target, e)
		}
	}
	stats, err := s.GetStatistics()
	if err != nil || stats.Instances != 2 || stats.Databases != 4 {
		t.Fatal(stats, err)
	}
	found := false
	for _, d := range stats.DBMS {
		if d.Name == "Oracle Database" {
			found = d.Instances == 0 && d.Databases == 2
		}
	}
	if !found {
		t.Fatal(stats.DBMS)
	}
	encoded, err := s.ExportExcel()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.StdEncoding.DecodeString(encoded)
	wb, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	exported, _ := wb.GetRows("Каталог")
	wb.Close()
	oracles := 0
	for _, r := range exported[1:] {
		if r[5] == "Oracle Database" {
			oracles++
			if r[1] != "" || r[6] != "19c" {
				t.Fatal(r)
			}
		}
	}
	if oracles != 2 {
		t.Fatal(exported)
	}
	preview, err = s.PreviewExcel(encoded)
	if err != nil || preview.Valid != 4 {
		t.Fatal(preview, err)
	}
	result, err = s.ApplyExcel(preview.Token, false, false)
	if err != nil || result.DatabasesCreated != 0 || result.ObjectsUpdated != 0 {
		t.Fatal("round trip changed catalog", result, err)
	}
	var server Object
	for _, o := range c.Servers {
		if o.Name == "host-a" {
			server = o
		}
	}
	del, err := s.PreviewDeletion("server", server.ID, server.Version)
	if err != nil || del.Databases != 3 || del.Instances != 2 || del.Policies != 1 {
		t.Fatal(del, err)
	}
	if _, err = s.DeleteObjectConfirmed("server", server.ID, server.Version, del.Fingerprint); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCatalog()
	if len(c.Databases) != 1 || c.Databases[0].DBMS != "Oracle Database" {
		t.Fatal(c)
	}
}
func TestImportExplainsSharedFieldsAndScopesDuplicateSIDs(t *testing.T) {
	s, _, _ := catalogFixture(t)
	a := oracleRow("host", "10.0.0.1", "A", "default", "Oracle Database")
	b := oracleRow("HOST", "10.0.0.2", "B", "default", "Oracle Database")
	b[15] = "Windows"
	preview, err := s.PreviewExcel(oracleWorkbook(t, [][21]string{a, b}))
	if err != nil || preview.Valid != 1 {
		t.Fatal(preview, err)
	}
	message := strings.Join(preview.Rows[1].Errors, ";")
	for _, part := range []string{"IPv4-адрес", "Операционная система", "строки 2 и 3"} {
		if !strings.Contains(message, part) {
			t.Fatal(message)
		}
	}
	b = a
	b[1] = "another-ignored-instance"
	preview, err = s.PreviewExcel(oracleWorkbook(t, [][21]string{a, b}))
	if err != nil || preview.Valid != 1 || !strings.Contains(strings.Join(preview.Rows[1].Errors, ";"), "сервера Oracle") {
		t.Fatal(preview, err)
	}
}
func TestOracleParentConstraintsAndPermissions(t *testing.T) {
	s, _, _ := catalogFixture(t)
	server, err := s.SaveObject(Object{Kind: "server", Name: "host"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveObject(Object{Kind: "instance", Name: "default", DBMS: "Oracle Database", ParentID: server.ID}); err == nil {
		t.Fatal("Oracle instance accepted")
	}
	port := 1521
	direct, err := s.SaveObject(Object{Kind: "database", Name: "COPY", DBMS: "Oracle Database", DBMSVersion: "21c", ParentID: server.ID, Port: &port})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Databases) != 1 || c.Databases[0].Port == nil || *c.Databases[0].Port != 1521 {
		t.Fatal("Oracle port lost", c, err)
	}
	if _, err = s.SaveObject(Object{Kind: "database", Name: "COPY", DBMS: "Oracle Database", ParentID: server.ID}); err == nil {
		t.Fatal("duplicate SID on same server")
	}
	inst, err := s.SaveObject(Object{Kind: "instance", Name: "pg", DBMS: "PostgreSQL", ParentID: server.ID})
	if err != nil {
		t.Fatal(err)
	}
	direct.ParentID = inst.ID
	if _, err = s.SaveObject(direct); err == nil {
		t.Fatal("Oracle accepted instance as parent")
	}
}
