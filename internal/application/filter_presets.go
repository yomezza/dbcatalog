package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/yomezza/test/internal/security"
	"strings"
	"time"
)

type CatalogCriteria struct {
	Query       string `json:"query"`
	DBMS        string `json:"dbms"`
	Status      string `json:"status"`
	Environment string `json:"environment"`
	Responsible string `json:"responsible"`
	Policy      string `json:"policy"`
}
type FilterPreset struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Version  int64           `json:"version"`
	Criteria CatalogCriteria `json:"criteria"`
}

func (s *Service) ListFilterPresets() ([]FilterPreset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id::text,name,version,criteria FROM saved_filters WHERE user_id=$1 ORDER BY lower(name),id`, s.user.ID)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	defer rows.Close()
	out := []FilterPreset{}
	for rows.Next() {
		var f FilterPreset
		var raw []byte
		if err = rows.Scan(&f.ID, &f.Name, &f.Version, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &f.Criteria); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Service) SaveFilterPreset(f FilterPreset) (FilterPreset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" || len([]rune(f.Name)) > 100 {
		return f, errors.New("Название набора: от 1 до 100 символов")
	}
	if len(f.Criteria.Query) > 2000 {
		return f, errors.New("Слишком длинный поисковый запрос")
	}
	for _, v := range []string{f.Criteria.DBMS, f.Criteria.Status, f.Criteria.Environment, f.Criteria.Responsible} {
		if len(v) > 4000 {
			return f, errors.New("Слишком длинное значение фильтра")
		}
	}
	switch f.Criteria.Policy {
	case "", "any", "active", "disabled", "missing", "present":
	default:
		return f, errors.New("Неизвестный фильтр политики РК")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return f, err
	}
	defer tx.Rollback(ctx)
	raw, _ := json.Marshal(f.Criteria)
	if f.ID == "" {
		f.ID = security.ID()
		f.Version = 1
		_, err = tx.Exec(ctx, `INSERT INTO saved_filters(id,user_id,name,criteria) VALUES($1,$2,$3,$4)`, f.ID, s.user.ID, f.Name, raw)
	} else {
		tag, e := tx.Exec(ctx, `UPDATE saved_filters SET name=$3,criteria=$4,version=version+1,updated_at=now() WHERE id=$1 AND user_id=$2 AND version=$5`, f.ID, s.user.ID, f.Name, raw, f.Version)
		err = e
		if err == nil && tag.RowsAffected() != 1 {
			return f, errors.New("Набор изменён или недоступен. Обновите список")
		}
		f.Version++
	}
	if err != nil {
		return f, importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return f, importDatabaseError(err)
	}
	return f, nil
}
func (s *Service) DeleteFilterPreset(id string, version int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM saved_filters WHERE id=$1 AND user_id=$2 AND version=$3`, id, s.user.ID, version)
	if err != nil {
		return importDatabaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("Набор изменён или недоступен")
	}
	return tx.Commit(ctx)
}
