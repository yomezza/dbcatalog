package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type DeletionPreview struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Servers     int    `json:"servers"`
	Instances   int    `json:"instances"`
	Databases   int    `json:"databases"`
	Policies    int    `json:"policies"`
	Fingerprint string `json:"fingerprint"`
}
type deletionPlan struct {
	preview                       DeletionPreview
	servers, instances, databases []Object
	policies                      []Policy
}

func deletionObjects(ctx context.Context, tx pgx.Tx, kind, condition string, args ...any) ([]Object, error) {
	t, err := table(kind)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT to_jsonb(t) FROM "+t+" t WHERE "+condition+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Object{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		o, e := decodeObject(raw, kind)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func objectIDs(objects []Object) []string {
	ids := []string{}
	for _, o := range objects {
		ids = append(ids, o.ID)
	}
	return ids
}
func loadDeletion(ctx context.Context, tx pgx.Tx, kind, id string, version int64) (deletionPlan, error) {
	plan := deletionPlan{preview: DeletionPreview{Kind: kind, Name: "Весь каталог"}}
	var err error
	if kind != "all" {
		t, e := table(kind)
		if e != nil {
			return plan, e
		}
		var raw []byte
		if err = tx.QueryRow(ctx, "SELECT to_jsonb(t) FROM "+t+" t WHERE id=$1", id).Scan(&raw); err != nil {
			return plan, errors.New("Объект не найден")
		}
		root, e := decodeObject(raw, kind)
		if e != nil {
			return plan, e
		}
		if root.Version != version {
			return plan, errors.New("Объект изменён другим пользователем. Обновите каталог")
		}
		plan.preview.Name = root.Name
		switch kind {
		case "server":
			plan.servers = []Object{root}
		case "instance":
			plan.instances = []Object{root}
		case "database":
			plan.databases = []Object{root}
		}
	} else {
		plan.servers, err = deletionObjects(ctx, tx, "server", "true")
		if err != nil {
			return plan, err
		}
	}
	if kind == "server" || kind == "all" {
		plan.instances, err = deletionObjects(ctx, tx, "instance", "server_id=ANY($1::uuid[])", objectIDs(plan.servers))
		if err != nil {
			return plan, err
		}
	}
	if kind != "database" {
		plan.databases, err = deletionObjects(ctx, tx, "database", "instance_id=ANY($1::uuid[]) OR server_id=ANY($2::uuid[])", objectIDs(plan.instances), objectIDs(plan.servers))
		if err != nil {
			return plan, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT to_jsonb(p) FROM backup_policies p WHERE instance_id=ANY($1::uuid[]) OR database_id=ANY($2::uuid[]) ORDER BY id`, objectIDs(plan.instances), objectIDs(plan.databases))
	if err != nil {
		return plan, err
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return plan, err
		}
		p, e := decodePolicy(raw)
		if e != nil {
			rows.Close()
			return plan, e
		}
		plan.policies = append(plan.policies, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return plan, err
	}
	plan.preview.Servers = len(plan.servers)
	plan.preview.Instances = len(plan.instances)
	plan.preview.Databases = len(plan.databases)
	plan.preview.Policies = len(plan.policies)
	type stamp struct {
		Kind, ID string
		Version  int64
	}
	stamps := []stamp{}
	for _, objects := range [][]Object{plan.servers, plan.instances, plan.databases} {
		for _, o := range objects {
			stamps = append(stamps, stamp{o.Kind, o.ID, o.Version})
		}
	}
	for _, p := range plan.policies {
		stamps = append(stamps, stamp{"policy", p.ID, p.Version})
	}
	var schema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return plan, err
	}
	raw, err := json.Marshal(struct {
		Schema  string
		Kind    string
		Objects []stamp
	}{schema, kind, stamps})
	if err != nil {
		return plan, err
	}
	plan.preview.Fingerprint = fmt.Sprintf("%x", sha256.Sum256(raw))
	return plan, nil
}
func (s *Service) PreviewDeletion(kind, id string, version int64) (DeletionPreview, error) {
	if _, err := table(kind); err != nil {
		return DeletionPreview{}, err
	}
	return s.previewDeletion(kind, id, version)
}
func (s *Service) PreviewClearCatalog() (DeletionPreview, error) {
	return s.previewDeletion("all", "", 0)
}
func (s *Service) previewDeletion(kind, id string, version int64) (DeletionPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, true)
	if err != nil {
		return DeletionPreview{}, err
	}
	defer tx.Rollback(ctx)
	plan, err := loadDeletion(ctx, tx, kind, id, version)
	if err != nil {
		return DeletionPreview{}, err
	}
	return plan.preview, nil
}

// DeleteObject is retained for internal use. The desktop bridge requires a preview fingerprint.
func (s *Service) DeleteObject(kind, id string, version int64) error {
	if _, err := table(kind); err != nil {
		return err
	}
	_, err := s.deleteTree(kind, id, version, "")
	return err
}
func (s *Service) DeleteObjectConfirmed(kind, id string, version int64, fingerprint string) (DeletionPreview, error) {
	if fingerprint == "" {
		return DeletionPreview{}, errors.New("Сначала проверьте состав удаляемой ветви")
	}
	if _, err := table(kind); err != nil {
		return DeletionPreview{}, err
	}
	return s.deleteTree(kind, id, version, fingerprint)
}
func (s *Service) ClearCatalog(fingerprint, confirmation string) (DeletionPreview, error) {
	if confirmation != "УДАЛИТЬ КАТАЛОГ" || fingerprint == "" {
		return DeletionPreview{}, errors.New("Подтвердите очистку каталога фразой УДАЛИТЬ КАТАЛОГ")
	}
	return s.deleteTree("all", "", 0, fingerprint)
}
func (s *Service) deleteTree(kind, id string, version int64, fingerprint string) (DeletionPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if s.pool == nil {
		return DeletionPreview{}, errors.New("Нет подключения")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeletionPreview{}, importDatabaseError(err)
	}
	defer tx.Rollback(ctx)
	if err = s.authorize(ctx, tx, true, true); err != nil {
		return DeletionPreview{}, err
	}
	// READ COMMITTED + table locks include children committed while waiting for the lock.
	if _, err = tx.Exec(ctx, `LOCK TABLE servers,instances,databases,backup_policies IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return DeletionPreview{}, importDatabaseError(err)
	}
	plan, err := loadDeletion(ctx, tx, kind, id, version)
	if err != nil {
		return DeletionPreview{}, err
	}
	if fingerprint != "" && fingerprint != plan.preview.Fingerprint {
		return DeletionPreview{}, errors.New("Состав или данные удаляемых записей изменились. Закройте подтверждение и проверьте состав заново")
	}
	if err = s.archiveDeletion(ctx, tx, plan); err != nil {
		return DeletionPreview{}, err
	}
	for _, p := range plan.policies {
		if err = history(ctx, tx, s.user.ID, "delete", "policy", p.ID, p, nil); err != nil {
			return DeletionPreview{}, importDatabaseError(err)
		}
		tag, e := tx.Exec(ctx, `DELETE FROM backup_policies WHERE id=$1`, p.ID)
		if e != nil {
			return DeletionPreview{}, importDatabaseError(e)
		}
		if tag.RowsAffected() != 1 {
			return DeletionPreview{}, errors.New("Политика не удалена; операция отменена")
		}
	}
	for _, objects := range [][]Object{plan.databases, plan.instances, plan.servers} {
		for _, o := range objects {
			if err = history(ctx, tx, s.user.ID, "delete", o.Kind, o.ID, o, nil); err != nil {
				return DeletionPreview{}, importDatabaseError(err)
			}
			t, _ := table(o.Kind)
			tag, e := tx.Exec(ctx, "DELETE FROM "+t+" WHERE id=$1", o.ID)
			if e != nil {
				return DeletionPreview{}, importDatabaseError(e)
			}
			if tag.RowsAffected() != 1 {
				return DeletionPreview{}, errors.New("Запись не удалена; операция отменена")
			}
		}
	}
	details, _ := json.Marshal(map[string]int{"servers": plan.preview.Servers, "instances": plan.preview.Instances, "databases": plan.preview.Databases, "policies": plan.preview.Policies})
	action := "cascade_delete"
	entity := kind
	var objectID any = id
	if kind == "all" {
		action = "catalog_clear"
		entity = "catalog"
		objectID = nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id,details) VALUES($1,$2,$3,$4,$5)`, s.user.ID, action, entity, objectID, details); err != nil {
		return DeletionPreview{}, importDatabaseError(err)
	}
	if kind == "all" {
		if _, err = tx.Exec(ctx, `INSERT INTO notifications(user_id,category,title,body,entity) SELECT id,'administration','Каталог очищен','Администратор удалил все записи каталога. История и пользователи сохранены.','catalog' FROM users WHERE state='active'`); err != nil {
			return DeletionPreview{}, importDatabaseError(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return DeletionPreview{}, importDatabaseError(err)
	}
	return plan.preview, nil
}
