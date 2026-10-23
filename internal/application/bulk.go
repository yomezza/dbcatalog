package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/security"
	"reflect"
	"sort"
	"time"
)

type ObjectRef struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type BulkPatch struct {
	SetResponsible    bool     `json:"setResponsible"`
	Responsible       string   `json:"responsible"`
	SetOrganization   bool     `json:"setOrganization"`
	Organization      string   `json:"organization"`
	SetStatus         bool     `json:"setStatus"`
	Status            string   `json:"status"`
	SetAdministration bool     `json:"setAdministration"`
	Administration    []string `json:"administration"`
}
type BulkChange struct {
	Before Object `json:"before"`
	After  Object `json:"after"`
}
type BulkPreview struct {
	Fingerprint string       `json:"fingerprint"`
	Selected    int          `json:"selected"`
	Changed     int          `json:"changed"`
	Items       []BulkChange `json:"items"`
}

func (s *Service) buildBulk(ctx context.Context, tx pgx.Tx, refs []ObjectRef, patch BulkPatch) (BulkPreview, error) {
	out := BulkPreview{Items: []BulkChange{}}
	if len(refs) == 0 || len(refs) > 10000 {
		return out, errors.New("Выберите от 1 до 10000 баз данных")
	}
	if !patch.SetResponsible && !patch.SetOrganization && !patch.SetStatus && !patch.SetAdministration {
		return out, errors.New("Выберите изменяемые поля")
	}
	expected := map[string]int64{}
	ids := []string{}
	for _, r := range refs {
		if r.ID == "" || r.Version < 1 || expected[r.ID] != 0 {
			return out, errors.New("Некорректный или повторный объект выбора")
		}
		expected[r.ID] = r.Version
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	choices, err := loadChoices(ctx, tx)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT (to_jsonb(d)-'search_text') FROM databases d WHERE id=ANY($1::uuid[]) ORDER BY id`, ids)
	if err != nil {
		return out, importDatabaseError(err)
	}
	objects := []Object{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return out, err
		}
		o, e := decodeObject(raw, "database")
		if e != nil {
			rows.Close()
			return out, e
		}
		objects = append(objects, o)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(objects) != len(ids) {
		return out, errors.New("Часть выбранных БД удалена. Обновите каталог")
	}
	for _, before := range objects {
		if expected[before.ID] != before.Version {
			return out, errors.New("Выбранная БД изменена другим пользователем. Обновите выбор")
		}
		after := before
		if patch.SetResponsible {
			after.Attributes.Responsible = patch.Responsible
		}
		if patch.SetOrganization {
			after.Attributes.ServiceOrganization = patch.Organization
		}
		if patch.SetStatus {
			if patch.Status == "" {
				return out, errors.New("Укажите состояние")
			}
			after.Status = patch.Status
		}
		if patch.SetAdministration {
			after.Attributes.Administration = append([]string{}, patch.Administration...)
		}
		after.Attributes, err = normalizeAttributes(after.Attributes)
		if err != nil {
			return out, err
		}
		if err = choices.validateObject(after, before); err != nil {
			return out, err
		}
		if !reflect.DeepEqual(before, after) {
			out.Items = append(out.Items, BulkChange{before, after})
		}
	}
	out.Selected = len(ids)
	out.Changed = len(out.Items)
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return out, err
	}
	raw, _ := json.Marshal(struct {
		Schema, User string
		IDs          []string
		Objects      []Object
		Patch        BulkPatch
	}{schema, s.user.ID, ids, objects, patch})
	out.Fingerprint = fmt.Sprintf("%x", sha256.Sum256(raw))
	return out, nil
}
func (s *Service) PreviewBulkEdit(refs []ObjectRef, patch BulkPatch) (BulkPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return BulkPreview{}, err
	}
	defer tx.Rollback(ctx)
	return s.buildBulk(ctx, tx, refs, patch)
}
func (s *Service) ApplyBulkEdit(refs []ObjectRef, patch BulkPatch, fingerprint string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fingerprint == "" {
		return 0, errors.New("Сначала проверьте предпросмотр")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if s.pool == nil {
		return 0, errors.New("Нет подключения")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, importDatabaseError(err)
	}
	defer tx.Rollback(ctx)
	if err = s.authorize(ctx, tx, true, false); err != nil {
		return 0, err
	}
	ids := []string{}
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM databases WHERE id=ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids); err != nil {
		return 0, importDatabaseError(err)
	}
	plan, err := s.buildBulk(ctx, tx, refs, patch)
	if err != nil {
		return 0, err
	}
	if plan.Fingerprint != fingerprint {
		return 0, errors.New("Предпросмотр изменился. Проверьте его заново")
	}
	changes := []Object{}
	for _, item := range plan.Items {
		changes = append(changes, item.After)
	}
	raw, err := json.Marshal(changes)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `SELECT api_bulk_update($1)`, raw); err != nil {
		return 0, importDatabaseError(err)
	}
	details, _ := json.Marshal(map[string]int{"selected": plan.Selected, "changed": plan.Changed})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id,details)VALUES($1,'bulk_update','catalog',$2,$3)`, s.user.ID, security.ID(), details); err != nil {
		return 0, importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, importDatabaseError(err)
	}
	return plan.Changed, nil
}
