package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaValidation(t *testing.T) {
	base := Connection{Host: "localhost", Database: "catalog", Username: "catalog"}.Normalize()
	if base.Schema != DefaultSchema || base.SSLMode != "verify-full" {
		t.Fatal("unsafe defaults")
	}
	for _, schema := range []string{"public", "pg_catalog", "pg_temp", "information_schema", "x;DROP SCHEMA public CASCADE", "a.b", "a\"b", "a b", "UpperCase", strings.Repeat("x", 64)} {
		c := base
		c.Schema = schema
		if c.Validate() == nil {
			t.Errorf("accepted unsafe schema %q", schema)
		}
	}
	for _, schema := range []string{"dbcatalog", "catalog_team_2", "_private"} {
		c := base
		c.Schema = schema
		if err := c.Validate(); err != nil {
			t.Errorf("rejected %q: %v", schema, err)
		}
	}
}
func TestConfigRoundTripAndLegacyDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "connection.json")
	c := Connection{Host: "localhost", Database: "catalog", Username: "user", Schema: "team_a"}.Normalize()
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, c); err != nil {
		t.Fatal("repeat save:", err)
	}
	got, err := Load(path)
	if err != nil || got != c {
		t.Fatalf("round trip: %#v %v", got, err)
	}
	if err = os.WriteFile(path, []byte(`{"host":"localhost","database":"legacy","username":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got.Schema != "dbcatalog" {
		t.Fatal("old configuration must default to dbcatalog", err)
	}
}

func TestCredentialIdentityIsUnambiguous(t *testing.T) {
	a := Connection{Host: "localhost", Database: "db/a", Username: "b", Schema: "one"}.Normalize()
	b := a
	b.Database = "db"
	b.Username = "a/b"
	if a.CredentialKey() == b.CredentialKey() {
		t.Fatal("credential identity collision")
	}
	b = a
	b.Schema = "two"
	if a.CredentialKey() == b.CredentialKey() {
		t.Fatal("credentials reused between schemas")
	}
}
