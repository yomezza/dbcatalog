// Package downloads keeps generated XLSX files in a managed, local, per-user workspace.
package downloads

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yomezza/test/internal/security"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

type Entry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"createdAt"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	Available bool      `json:"available"`
}

var idPattern = regexp.MustCompile(`^[a-f0-9-]{36}$`)

type Store struct{ Root string }

func (s Store) Add(name, category string, data []byte) (Entry, error) {
	e := Entry{ID: security.ID(), Name: filepath.Base(name), Category: category, CreatedAt: time.Now().UTC(), Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Available: true}
	if filepath.Ext(e.Name) != ".xlsx" {
		return Entry{}, errors.New("Поддерживаются только файлы XLSX")
	}
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return Entry{}, err
	}
	path := filepath.Join(s.Root, e.ID+".xlsx")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return Entry{}, err
	}
	raw, err := json.Marshal(e)
	if err == nil {
		err = os.WriteFile(filepath.Join(s.Root, e.ID+".json"), raw, 0600)
	}
	if err != nil {
		os.Remove(path)
		return Entry{}, err
	}
	return e, nil
}
func (s Store) List() ([]Entry, error) {
	result := []Entry{}
	files, err := os.ReadDir(s.Root)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		id := f.Name()[:len(f.Name())-5]
		if !idPattern.MatchString(id) {
			continue
		}
		e, err := s.entry(id)
		if err != nil {
			return nil, errors.New("Не удалось прочитать историю загрузок")
		}
		info, err := os.Lstat(filepath.Join(s.Root, id+".xlsx"))
		e.Available = err == nil && info.Mode().IsRegular() && info.Size() == e.Size
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}
func (s Store) entry(id string) (Entry, error) {
	var e Entry
	if !idPattern.MatchString(id) {
		return e, errors.New("Некорректный идентификатор файла")
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, id+".json"))
	if err != nil {
		return e, errors.New("Файл отсутствует в истории загрузок")
	}
	if err = json.Unmarshal(raw, &e); err != nil || e.ID != id || filepath.Base(e.Name) != e.Name || filepath.Ext(e.Name) != ".xlsx" {
		return e, errors.New("Повреждены сведения о загрузке")
	}
	return e, nil
}
func (s Store) Resolve(id string) (Entry, string, error) {
	e, err := s.entry(id)
	if err != nil {
		return e, "", err
	}
	path := filepath.Join(s.Root, id+".xlsx")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return e, "", errors.New("Сохранённый файл недоступен")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return e, "", errors.New("Не удалось прочитать сохранённый файл")
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != e.SHA256 {
		return e, "", errors.New("Файл изменён вне приложения; проверка целостности не пройдена")
	}
	return e, path, nil
}
