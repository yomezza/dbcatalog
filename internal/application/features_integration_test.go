package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"
	"os"
	"strings"
	"testing"
	"time"
)

func TestImportCorrectionsRevalidateAllAndRotateToken(t *testing.T) {
	s, _, _ := catalogFixture(t)
	preview, err := s.PreviewExcel(importFixture(t, 28, "one", "one"))
	if err != nil || preview.Valid != 1 {
		t.Fatal(preview, err)
	}
	values := append([]string{}, preview.Rows[1].Values...)
	values[0] = "two"
	next, err := s.RevalidateExcel(preview.Token, []ImportRowEdit{{Row: 3, Values: values}})
	if err != nil || next.Valid != 2 || next.Token == preview.Token {
		t.Fatal(next, err)
	}
	if _, err = s.ApplyExcel(preview.Token, true, true); err == nil {
		t.Fatal("old token accepted")
	}
	if _, err = s.RevalidateExcel(next.Token, []ImportRowEdit{{Row: 999, Values: values}}); err == nil {
		t.Fatal("unknown row accepted")
	}
	c, err := s.GetCatalog()
	if err != nil || len(c.Databases) != 0 {
		t.Fatal("correction wrote catalog", err)
	}
	changed := append([]string{}, values...)
	changed[9] = "10.9.9.9"
	conflict, err := s.RevalidateExcel(next.Token, []ImportRowEdit{{Row: 3, Values: changed}})
	if err != nil || conflict.Valid != 1 {
		t.Fatal("shared server conflict missed", conflict, err)
	}
	next, err = s.RevalidateExcel(conflict.Token, []ImportRowEdit{{Row: 3, Values: values}})
	if err != nil || next.Valid != 2 {
		t.Fatal(err)
	}
	file, err := s.ExportImportPreview(next.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, path, err := s.ResolveDownload(file.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	rows, err := wb.GetRows("Каталог")
	if err != nil || rows[2][0] != "two" || len(rows[0]) != 21 {
		t.Fatal("corrected workbook mismatch", err)
	}
	raw, _ := json.Marshal(next)
	if strings.Contains(string(raw), "PASSWORD") {
		t.Fatal("secret leakage")
	}
	result, err := s.ApplyExcel(next.Token, false, true)
	if err != nil || result.DatabasesCreated != 2 {
		t.Fatal(result, err)
	}
}
func TestNotificationsRecipientsReadStateAndCleanup(t *testing.T) {
	s, p, schema := catalogFixture(t)
	admin := *s.user
	ctx := context.Background()
	if err := s.Register("seconduser", "long-enough-password", false); err != nil {
		t.Fatal(err)
	}
	inbox, err := s.GetNotifications(0, false)
	if err != nil || inbox.Unread != 1 || inbox.Items[0].Entity != "user" {
		t.Fatal(inbox, err)
	}
	users, err := s.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	var second User
	for _, u := range users {
		if u.Login == "seconduser" {
			second = u
		}
	}
	if err = s.UpdateUser(second.ID, "viewer", "active"); err != nil {
		t.Fatal(err)
	}
	server, err := s.SaveObject(Object{Kind: "server", Name: "notify-host", Attributes: Attributes{ConnectionMethod: "SSH"}})
	if err != nil {
		t.Fatal(err)
	}
	s.user = &second
	if err = s.SetFavorite("server", server.ID, true); err != nil {
		t.Fatal(err)
	}
	inbox, err = s.GetNotifications(0, true)
	if err != nil || inbox.Unread != 1 || inbox.Items[0].Category != "account" {
		t.Fatal(inbox, err)
	}
	ownID := inbox.Items[0].ID
	s.user = &admin
	adminInbox, _ := s.GetNotifications(0, true)
	s.user = &second
	if err = s.MarkNotificationsRead([]string{adminInbox.Items[0].ID}, false); err != nil {
		t.Fatal(err)
	}
	s.user = &admin
	still, _ := s.GetNotifications(0, true)
	if still.Unread != 1 {
		t.Fatal("other user's notification marked read")
	}
	server.Description = "new"
	server, err = s.SaveObject(server)
	if err != nil {
		t.Fatal(err)
	}
	s.user = &second
	inbox, err = s.GetNotifications(0, true)
	if err != nil || inbox.Unread != 2 {
		t.Fatal(inbox, err)
	}
	if err = s.MarkNotificationsRead([]string{ownID}, false); err != nil {
		t.Fatal(err)
	}
	s.user = &admin
	// A forced transaction failure must not publish a notification.
	q := pgx.Identifier{schema}.Sanitize()
	_, err = p.Exec(ctx, `CREATE FUNCTION `+q+`.reject_server() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test'; END $$; CREATE TRIGGER reject_server BEFORE DELETE ON `+q+`.servers FOR EACH ROW EXECUTE FUNCTION `+q+`.reject_server()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteObject("server", server.ID, server.Version); err == nil {
		t.Fatal("forced failure ignored")
	}
	s.user = &second
	inbox, _ = s.GetNotifications(0, true)
	if inbox.Unread != 1 {
		t.Fatal("rolled back notification persisted")
	}
	s.user = &admin
	p.Exec(ctx, `DROP TRIGGER reject_server ON `+q+`.servers`)
	preview, err := s.PreviewClearCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClearCatalog(preview.Fingerprint, "УДАЛИТЬ КАТАЛОГ"); err != nil {
		t.Fatal(err)
	}
	s.user = &second
	inbox, err = s.GetNotifications(0, true)
	if err != nil || inbox.Unread != 3 {
		t.Fatal("favorite delete and cleanup notifications missing", inbox, err)
	}
	if err = s.MarkNotificationsRead(nil, true); err != nil {
		t.Fatal(err)
	}
	inbox, _ = s.GetNotifications(0, true)
	if inbox.Unread != 0 || len(inbox.Items) != 0 {
		t.Fatal(inbox)
	}
}
func TestStatisticsExportAndDownloadAuthorization(t *testing.T) {
	s, _, _ := catalogFixture(t)
	srv, inst, dbs, policies := seedDeletionBranch(t, s, "Host")
	if _, err := s.SaveObject(Object{Kind: "server", Name: " host ", Address: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	active := policies[0]
	active.Enabled = true
	active.BackupType = "Локальное"
	active.Scheduler = "cron"
	active.Schedule = "Ежедневно"
	active.Storage = "/backup"
	active.RetentionDays = 7
	if _, err := s.SavePolicy(active); err != nil {
		t.Fatal(err)
	}
	stats, err := s.GetStatistics()
	if err != nil || stats.Servers != 2 || stats.UniqueServers != 1 || stats.ServerAddresses != 2 || stats.ProtectedDatabases != 1 || stats.DisabledDatabases != 1 || stats.UnprotectedDatabases != 0 {
		t.Fatal(stats, err)
	}
	if stats.Instances != 1 || stats.Databases != 2 || len(stats.DBMS) != 1 || stats.DBMS[0].Databases != 2 {
		t.Fatal(stats)
	}
	srv.Attributes.ConnectionMethod = "RDP"
	srv, err = s.SaveObject(srv)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []Object{srv, inst, dbs[0]} {
		target, e := s.GetServerConnection(o.Kind, o.ID)
		if e != nil || target.Method != "RDP" || target.Host != "Host" {
			t.Fatal(target, e)
		}
	}
	file, err := s.ExportToDownloads()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.ListDownloads()
	if err != nil || len(entries) != 1 || entries[0].ID != file.ID {
		t.Fatal(entries, err)
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
	defer wb.Close()
	summary, err := wb.GetCellValue("Статистика", "B2")
	if err != nil || summary != "2" {
		t.Fatal(summary, err)
	}
	if err = s.Register("otherdownloader", "long-enough-password", false); err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	var other User
	for _, u := range users {
		if u.Login == "otherdownloader" {
			other = u
		}
	}
	if err = s.UpdateUser(other.ID, "viewer", "active"); err != nil {
		t.Fatal(err)
	}
	s.user = &other
	if _, _, err = s.ResolveDownload(file.ID); err == nil {
		t.Fatal("another user accessed a private download")
	}
	own, err := s.ListDownloads()
	if err != nil || len(own) != 0 {
		t.Fatal("cross-user download history", own, err)
	}
	s.user = nil
	if _, _, err = s.ResolveDownload(file.ID); err == nil {
		t.Fatal("unauthenticated download")
	}
}
func TestServerHostsRejectCommands(t *testing.T) {
	for _, host := range []string{"host", "host.example.org", "10.0.0.1", "2001:db8::1"} {
		if !validateServerHost(host) {
			t.Fatal(host)
		}
	}
	for _, host := range []string{"-oProxyCommand=whoami", "host & calc", "user@host", "host/../../x", "host..test", "bad\nname"} {
		if validateServerHost(host) {
			t.Fatal(host)
		}
	}
}
func TestImportCorrectionPermissionAndExpiry(t *testing.T) {
	s, pool, schema := catalogFixture(t)
	preview, err := s.PreviewExcel(importFixture(t, 21, "one"))
	if err != nil {
		t.Fatal(err)
	}
	values := preview.Rows[0].Values
	q := pgx.Identifier{schema}.Sanitize()
	if _, err = pool.Exec(context.Background(), "UPDATE "+q+".users SET role='viewer' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RevalidateExcel(preview.Token, []ImportRowEdit{{Row: 2, Values: values}}); err == nil {
		t.Fatal("demoted user revised an import")
	}
	if s.pendingImport.preview.Token != preview.Token {
		t.Fatal("rejected revision changed the plan")
	}
	if _, err = pool.Exec(context.Background(), "UPDATE "+q+".users SET role='admin' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	s.pendingImport.expires = time.Now().Add(-time.Second)
	if _, err = s.RevalidateExcel(preview.Token, []ImportRowEdit{{Row: 2, Values: values}}); err == nil {
		t.Fatal("expired preview accepted")
	}
}
