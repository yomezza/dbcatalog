package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/test/internal/application"
	"github.com/yomezza/test/internal/config"
	"github.com/yomezza/test/internal/security"
)

type memoryCredentials map[string]string

func (s memoryCredentials) Get(service, key string) (string, error) {
	v, ok := s[service+":"+key]
	if !ok {
		return "", errors.New("catalogdb.secret.not-found")
	}
	return v, nil
}
func (s memoryCredentials) Set(service, key, value string) error {
	s[service+":"+key] = value
	return nil
}
func newClient(t *testing.T) *Client {
	t.Helper()
	c, e := NewClient(filepath.Join(t.TempDir(), "connection.json"), memoryCredentials{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(c.Close)
	return c
}
func invoke(t *testing.T, c *Client, name string, args []any, out any) string {
	t.Helper()
	raw, e := json.Marshal(args)
	if e != nil {
		t.Fatal(e)
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if e = json.Unmarshal([]byte(c.Call(name, string(raw))), &result); e != nil {
		t.Fatal(e)
	}
	if result.Error == "" && out != nil {
		if e = json.Unmarshal(result.Result, out); e != nil {
			t.Fatal(e)
		}
	}
	return result.Error
}
func TestRPCRejectsUnexposedMethodsAndInvalidArguments(t *testing.T) {
	c := newClient(t)
	for _, request := range []struct{ name, args string }{
		{"Close", "[]"}, {"SetChangeHandler", "[]"}, {"ResolveDownload", "[\"id\"]"}, {"GetServerConnection", "[]"},
		{"GetConnection", "{}"}, {"GetConnection", "null"}, {"GetConnection", "[1]"}, {"Connect", "[{},null,false,false]"},
		{"GetHistory", "[3,\"id\"]"}, {"RestoreVersion", "[\"database\",\"id\",1.5,2]"},
	} {
		response := c.Call(request.name, request.args)
		var out map[string]any
		if e := json.Unmarshal([]byte(response), &out); e != nil || out["error"] == nil {
			t.Fatalf("request was accepted: %s", request.name)
		}
	}
	var connection application.SavedConnection
	if e := invoke(t, c, "GetConnection", []any{}, &connection); e != "" || connection.HasPassword || connection.Connection.Schema != "dbcatalog" {
		t.Fatal("default connection", e, connection)
	}
	var d application.Diagnostics
	if e := invoke(t, c, "GetDiagnostics", []any{}, &d); e != "" || d.DatabaseState != "disconnected" || d.CredentialState != "unexamined" {
		t.Fatal("offline diagnostics", e, d)
	}
	_, missing := (nativeCredentials{memoryCredentials{}}).Get("CatalogDB", "missing")
	if !os.IsNotExist(missing) {
		t.Fatal("native missing credential was not normalized")
	}
	c.Close()
	if e := invoke(t, c, "GetConnection", []any{}, nil); e == "" {
		t.Fatal("closed client remained callable")
	}
}
func TestUIContractHasEveryBackendMethod(t *testing.T) {
	c := newClient(t)
	raw, e := os.ReadFile("../frontend/src/api.ts")
	if e != nil {
		t.Fatal(e)
	}
	source := strings.SplitN(strings.SplitN(string(raw), "interface Backend {", 2)[1], "declare global", 2)[0]
	native := map[string]bool{"OpenDownload": true, "ShowDownload": true, "SaveDownloadAs": true, "ConnectToServer": true, "IsSetupMode": true, "SaveDiagnosticsReport": true}
	for _, match := range regexp.MustCompile(`(?m)^  ([A-Z][A-Za-z0-9]+)\(`).FindAllStringSubmatch(source, -1) {
		if !native[match[1]] && !c.methods[match[1]].IsValid() {
			t.Error("Android backend is missing", match[1])
		}
	}
}

type events chan [2]string

func (e events) Emit(schema, kind string) {
	select {
	case e <- [2]string{schema, kind}:
	default:
	}
}
func TestRPCPostgreSQLCatalogRolesAndLiveEvents(t *testing.T) {
	dsn := os.Getenv("CATALOGDB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	schema := "mobile_" + strings.ReplaceAll(security.ID(), "-", "")
	defer func() {
		if _, e := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); e != nil {
			t.Error(e)
		}
	}()
	pc := pool.Config().ConnConfig
	cfig := config.Connection{Host: pc.Host, Port: pc.Port, Database: pc.Database, Username: pc.User, Schema: schema, SSLMode: "disable"}
	sink := make(events, 20)
	c, e := NewClient(filepath.Join(t.TempDir(), "connection.json"), memoryCredentials{}, sink)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if err := invoke(t, c, "Connect", []any{cfig, pc.Password, false, false}, nil); err != "" {
		t.Fatal(err)
	}
	password := security.ID()
	if err := invoke(t, c, "Register", []any{"tablet-admin", password, true}, nil); err != "" {
		t.Fatal(err)
	}
	if err := invoke(t, c, "Login", []any{"tablet-admin", password}, nil); err != "" {
		t.Fatal(err)
	}
	var server application.Object
	if err := invoke(t, c, "SaveObject", []any{application.Object{Kind: "server", Name: "tablet-server"}}, &server); err != "" {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
listening:
	for {
		select {
		case change := <-sink:
			if change == [2]string{schema, "catalog_changed"} {
				break listening
			}
		case <-deadline:
			t.Fatal("catalog change was not forwarded to mobile")
		}
	}
	var export struct {
		ID string `json:"id"`
	}
	if err := invoke(t, c, "ExportToDownloads", []any{}, &export); err != "" {
		t.Fatal(err)
	}
	info, e := c.DownloadInfo(export.ID)
	if e != nil || !strings.Contains(info, ".xlsx") {
		t.Fatal("native download lookup", e)
	}
	if err := invoke(t, c, "ExportObjectPDF", []any{"server", server.ID}, &export); err != "" {
		t.Fatal("mobile PDF export", err)
	}
	info, e = c.DownloadInfo(export.ID)
	if e != nil || !strings.Contains(info, "application/pdf") || !strings.Contains(info, ".pdf") {
		t.Fatal("mobile PDF MIME", info, e)
	}
	if err := invoke(t, c, "Register", []any{"tablet-viewer", password, false}, nil); err != "" {
		t.Fatal(err)
	}
	var users []application.User
	if err := invoke(t, c, "ListUsers", []any{}, &users); err != "" {
		t.Fatal(err)
	}
	for _, user := range users {
		if user.Login == "tablet-viewer" {
			if err := invoke(t, c, "UpdateUser", []any{user.ID, "viewer", "active"}, nil); err != "" {
				t.Fatal(err)
			}
		}
	}
	viewer, e := NewClient(filepath.Join(t.TempDir(), "connection.json"), memoryCredentials{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer viewer.Close()
	if err := invoke(t, viewer, "Connect", []any{cfig, pc.Password, false, false}, nil); err != "" {
		t.Fatal(err)
	}
	if err := invoke(t, viewer, "Login", []any{"tablet-viewer", password}, nil); err != "" {
		t.Fatal(err)
	}
	var catalog application.Catalog
	if err := invoke(t, viewer, "GetCatalog", []any{}, &catalog); err != "" || len(catalog.Servers) != 1 {
		t.Fatal("viewer read", err)
	}
	if err := invoke(t, viewer, "SaveObject", []any{application.Object{Kind: "server", Name: "forbidden"}}, nil); err == "" {
		t.Fatal("viewer acquired write permissions through mobile")
	}
}
