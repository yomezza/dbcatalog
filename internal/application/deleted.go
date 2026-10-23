package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

type DeletedRecord struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	BatchID   string          `json:"batchId"`
	Name      string          `json:"name"`
	DeletedAt time.Time       `json:"deletedAt"`
	Actor     string          `json:"actor"`
	Snapshot  json.RawMessage `json:"snapshot"`
}
type DeletedPage struct {
	Items []DeletedRecord `json:"items"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
}
type RestorePreview struct {
	Items       []DeletedRecord `json:"items"`
	Conflicts   []string        `json:"conflicts"`
	Fingerprint string          `json:"fingerprint"`
}

func (s *Service) ListDeleted(page int) (DeletedPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out := DeletedPage{Items: []DeletedRecord{}}
	tx, err := s.readTransaction(ctx, false, true)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM deleted_objects WHERE restored_at IS NULL`).Scan(&out.Total); err != nil {
		return out, importDatabaseError(err)
	}
	out.Page = boundedPage(page, out.Total, 25)
	out.Items, err = readDeleted(ctx, tx, `d.restored_at IS NULL ORDER BY d.deleted_at DESC,d.entity,d.object_id LIMIT 25 OFFSET $1`, out.Page*25)
	return out, err
}
func readDeleted(ctx context.Context, tx pgx.Tx, condition string, args ...any) ([]DeletedRecord, error) {
	rows, err := tx.Query(ctx, `SELECT d.entity,d.object_id::text,d.batch_id::text,coalesce(d.snapshot->>'name',''),d.deleted_at,u.login,d.snapshot FROM deleted_objects d JOIN users u ON u.id=d.deleted_by WHERE `+condition, args...)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	defer rows.Close()
	out := []DeletedRecord{}
	for rows.Next() {
		var r DeletedRecord
		if err = rows.Scan(&r.Kind, &r.ID, &r.BatchID, &r.Name, &r.DeletedAt, &r.Actor, &r.Snapshot); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) restoreDeletedPlan(ctx context.Context, tx pgx.Tx, kind, id string, branch bool) (RestorePreview, error) {
	out := RestorePreview{Items: []DeletedRecord{}, Conflicts: []string{}}
	if _, err := versionTable(kind); err != nil {
		return out, err
	}
	roots, err := readDeleted(ctx, tx, `d.entity=$1 AND d.object_id=$2 AND d.restored_at IS NULL`, kind, id)
	if err != nil {
		return out, err
	}
	if len(roots) != 1 {
		return out, errors.New("Удалённая запись не найдена")
	}
	all, err := readDeleted(ctx, tx, `d.batch_id=$1 AND d.restored_at IS NULL ORDER BY CASE d.entity WHEN 'server' THEN 0 WHEN 'instance' THEN 1 WHEN 'database' THEN 2 ELSE 3 END,d.object_id`, roots[0].BatchID)
	if err != nil {
		return out, err
	}
	selected := map[string]bool{kind + ":" + id: true}
	if branch {
		for pass := 0; pass < 3; pass++ {
			for _, r := range all {
				var o struct {
					ParentID string `json:"parentId"`
					ObjectID string `json:"objectId"`
					Kind     string `json:"kind"`
					DBMS     string `json:"dbms"`
				}
				if err = json.Unmarshal(r.Snapshot, &o); err != nil {
					return out, err
				}
				parentKind := "server"
				if r.Kind == "database" && o.DBMS != "Oracle Database" {
					parentKind = "instance"
				}
				parentID := o.ParentID
				if r.Kind == "policy" {
					parentKind = o.Kind
					parentID = o.ObjectID
				}
				if selected[parentKind+":"+parentID] {
					selected[r.Kind+":"+r.ID] = true
				}
			}
		}
	}
	for _, r := range all {
		if selected[r.Kind+":"+r.ID] {
			out.Items = append(out.Items, r)
		}
	}
	// Track selected records, their parents and name conflicts, not unrelated catalog changes.
	var stamps []byte
	relevant := []string{}
	names := []string{}
	for _, item := range out.Items {
		relevant = append(relevant, item.ID)
		var ref struct {
			ParentID string `json:"parentId"`
			ObjectID string `json:"objectId"`
			Name     string `json:"name"`
		}
		if err = json.Unmarshal(item.Snapshot, &ref); err != nil {
			return out, err
		}
		if ref.ParentID != "" {
			relevant = append(relevant, ref.ParentID)
		}
		if ref.ObjectID != "" {
			relevant = append(relevant, ref.ObjectID)
		}
		names = append(names, ref.Name)
	}
	err = tx.QueryRow(ctx, `WITH ids AS(SELECT unnest($1::uuid[])id UNION SELECT server_id FROM instances WHERE id=ANY($1::uuid[])),
 relevant AS(
 SELECT entity kind,id,version FROM v_catalog_objects WHERE id IN(SELECT id FROM ids) OR(parent_id IN(SELECT id FROM ids) AND name=ANY($2::text[]))
 OR(entity='server' AND lower(btrim(name)) IN(SELECT lower(btrim(n)) FROM unnest($2::text[])n))
 UNION ALL SELECT 'policy',id,version FROM backup_policies WHERE id=ANY($1::uuid[]) OR instance_id IN(SELECT id FROM ids) OR database_id IN(SELECT id FROM ids)
 ) SELECT coalesce(jsonb_agg(x ORDER BY kind,id),'[]')FROM relevant x`, relevant, names).Scan(&stamps)
	if err != nil {
		return out, err
	}
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return out, err
	}
	raw, _ := json.Marshal([]any{schema, s.user.ID, out.Items, stamps})
	out.Fingerprint = fmt.Sprintf("%x", sha256.Sum256(raw))
	return out, nil
}
func (s *Service) restoreDeletedRecords(ctx context.Context, tx pgx.Tx, p RestorePreview) error {
	refs := []map[string]string{}
	for _, r := range p.Items {
		refs = append(refs, map[string]string{"kind": r.Kind, "id": r.ID})
	}
	raw, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT api_restore_records($1)`, raw)
	return importDatabaseError(err)
}
func (s *Service) PreviewDeletedRestore(kind, id string, branch bool) (RestorePreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, true)
	if err != nil {
		return RestorePreview{}, err
	}
	defer tx.Rollback(ctx)
	p, err := s.restoreDeletedPlan(ctx, tx, kind, id, branch)
	if err != nil {
		return p, err
	}
	// Execute the exact operation in a rolled-back transaction: all FK, choice and unique checks apply.
	if err = s.restoreDeletedRecords(ctx, tx, p); err != nil {
		p.Conflicts = append(p.Conflicts, err.Error())
	}
	return p, nil
}
func (s *Service) RestoreDeleted(kind, id string, branch bool, fingerprint string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fingerprint == "" {
		return 0, errors.New("Сначала проверьте восстановление")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if s.pool == nil {
		return 0, errors.New("Нет подключения")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err = s.authorize(ctx, tx, true, true); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE servers,instances,databases,backup_policies,deleted_objects IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return 0, importDatabaseError(err)
	}
	p, err := s.restoreDeletedPlan(ctx, tx, kind, id, branch)
	if err != nil {
		return 0, err
	}
	if fingerprint != p.Fingerprint {
		return 0, errors.New("Состав восстановления или каталог изменились. Проверьте предпросмотр заново")
	}
	if err = s.restoreDeletedRecords(ctx, tx, p); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, importDatabaseError(err)
	}
	return len(p.Items), nil
}
