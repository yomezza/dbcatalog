package downloads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistenceIntegrityAndIsolation(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: filepath.Join(root, "first")}
	other := Store{Root: filepath.Join(root, "other")}
	e, err := s.Add("Каталог.xlsx", "Экспорт", []byte("xlsx-data"))
	if err != nil {
		t.Fatal(err)
	}
	reopened := Store{Root: s.Root}
	items, err := reopened.List()
	if err != nil || len(items) != 1 || !items[0].Available {
		t.Fatal(items, err)
	}
	if _, _, err = other.Resolve(e.ID); err == nil {
		t.Fatal("cross-scope access")
	}
	if _, _, err = s.Resolve("../../connection.json"); err == nil {
		t.Fatal("path traversal")
	}
	_, path, err := s.Resolve(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Resolve(e.ID); err == nil {
		t.Fatal("changed file accepted")
	}
	os.Remove(path)
	items, err = s.List()
	if err != nil || len(items) != 1 || items[0].Available {
		t.Fatal("missing files must retain history", items, err)
	}
}
