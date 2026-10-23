package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yomezza/test/internal/config"
)

type diagnosticTrapStore struct{ calls int }

func (s *diagnosticTrapStore) Get(string, string) (string, error) {
	s.calls++
	return "DIAGNOSTIC_SECRET_SENTINEL", nil
}
func (s *diagnosticTrapStore) Set(string, string, string) error { s.calls++; return nil }
func TestDiagnosticsOfflineDoesNotReadCredentialsOrExposeSettings(t *testing.T) {
	store := &diagnosticTrapStore{}
	path := filepath.Join(t.TempDir(), "connection.json")
	s := New(path, store)
	defer s.Close()
	d := s.GetDiagnostics()
	if d.DatabaseState != "disconnected" || d.ConfigState != "missing" || d.CredentialState != "unexamined" {
		t.Fatalf("unexpected offline report: %+v", d)
	}
	c := config.Connection{Host: "PRIVATE_HOST_SENTINEL", Port: 5432, Database: "PRIVATE_DATABASE_SENTINEL", Username: "PRIVATE_LOGIN_SENTINEL", Schema: "diagnostics", SSLMode: "disable"}
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	d = s.GetDiagnostics()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if d.ConfigState != "valid" || store.calls != 0 {
		t.Fatal("diagnostics probed credentials or rejected valid config")
	}
	for _, value := range []string{"SENTINEL", path, "connectionKey", "sessionID", "token", "password"} {
		if strings.Contains(string(raw), value) {
			t.Fatalf("sensitive field in report: %s", value)
		}
	}
	if err := os.WriteFile(path, []byte("invalid PRIVATE_CONFIG_SENTINEL"), 0600); err != nil {
		t.Fatal(err)
	}
	if s.GetDiagnostics().ConfigState != "error" {
		t.Fatal("invalid config not reported")
	}
}
