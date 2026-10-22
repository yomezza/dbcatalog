package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

type Movement struct {
	ID         int64     `json:"id"`
	FromPath   string    `json:"fromPath"`
	ToPath     string    `json:"toPath"`
	Reason     string    `json:"reason"`
	Version    int64     `json:"version"`
	Actor      string    `json:"actor"`
	CreatedAt  time.Time `json:"createdAt"`
	FromPolicy *Policy   `json:"fromPolicy"`
	ToPolicy   *Policy   `json:"toPolicy"`
}
type MovementPage struct {
	Items []Movement `json:"items"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
}
type MoveRequest struct {
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Version       int64  `json:"version"`
	ParentID      string `json:"parentId"`
	ParentVersion int64  `json:"parentVersion"`
	Reason        string `json:"reason"`
}
type MovePreview struct {
	FromDBMS    string   `json:"fromDBMS"`
	ToDBMS      string   `json:"toDBMS"`
	Object      Object   `json:"object"`
	Destination Object   `json:"destination"`
	FromPath    string   `json:"fromPath"`
	ToPath      string   `json:"toPath"`
	FromPolicy  *Policy  `json:"fromPolicy"`
	ToPolicy    *Policy  `json:"toPolicy"`
	Databases   int      `json:"databases"`
	Attachments int      `json:"attachments"`
	Fingerprint string   `json:"fingerprint"`
	Conflicts   []string `json:"conflicts"`
}

func objectPathTx(ctx context.Context, tx pgx.Tx, o Object) (string, error) {
	if o.Kind == "server" {
		return o.Name, nil
	}
	if o.Kind == "instance" || o.DBMS == "Oracle Database" {
		s, err := loadObject(ctx, tx, "server", o.ParentID, false)
		if err != nil {
			return "", err
		}
		middle := ""
		if o.Kind == "database" {
			middle = "Oracle Database / "
		}
		return s.Name + " / " + middle + o.Name, nil
	}
	i, err := loadObject(ctx, tx, "instance", o.ParentID, false)
	if err != nil {
		return "", err
	}
	path, err := objectPathTx(ctx, tx, i)
	return path + " / " + o.Name, err
}
func effectiveForObject(ctx context.Context, tx pgx.Tx, o Object) (*Policy, error) {
	var databaseID, instanceID any
	if o.Kind == "database" {
		databaseID = o.ID
		if o.DBMS != "Oracle Database" {
			instanceID = o.ParentID
		}
	} else if o.Kind == "instance" {
		instanceID = o.ID
	} else {
		return nil, nil
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM backup_policies p WHERE database_id=$1 OR instance_id=$2 ORDER BY(database_id IS NOT NULL) DESC LIMIT 1`, databaseID, instanceID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, err := decodePolicy(raw)
	return &p, err
}
func recordMovement(ctx context.Context, tx pgx.Tx, actor string, before, after Object, reason string) error {
	from, err := objectPathTx(ctx, tx, before)
	if err != nil {
		return err
	}
	to, err := objectPathTx(ctx, tx, after)
	if err != nil {
		return err
	}
	fp, err := effectiveForObject(ctx, tx, before)
	if err != nil {
		return err
	}
	tp, err := effectiveForObject(ctx, tx, after)
	if err != nil {
		return err
	}
	var fromPolicy, toPolicy any
	if fp != nil {
		fromPolicy, _ = json.Marshal(fp)
	}
	if tp != nil {
		toPolicy, _ = json.Marshal(tp)
	}
	_, err = tx.Exec(ctx, `INSERT INTO object_movements(entity,object_id,from_parent,to_parent,from_path,to_path,reason,from_policy,to_policy,object_version,actor_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, after.Kind, after.ID, before.ParentID, after.ParentID, from, to, reason, fromPolicy, toPolicy, after.Version, actor)
	if err != nil {
		return importDatabaseError(err)
	}
	if after.Kind == "instance" {
		_, err = tx.Exec(ctx, `WITH children AS(
 SELECT d.id,d.version,
 coalesce(to_jsonb(dp),to_jsonb(ip))||jsonb_build_object('kind',CASE WHEN dp.id IS NOT NULL THEN 'database' ELSE 'instance' END,'objectId',coalesce(dp.database_id,ip.instance_id),'backupType',coalesce(dp.backup_type,ip.backup_type),'retentionDays',coalesce(dp.retention_days,ip.retention_days)) AS policy,d.name
 FROM databases d LEFT JOIN backup_policies dp ON dp.database_id=d.id LEFT JOIN backup_policies ip ON ip.instance_id=d.instance_id WHERE d.instance_id=$1
 ) INSERT INTO object_movements(entity,object_id,from_parent,to_parent,from_path,to_path,reason,from_policy,to_policy,object_version,actor_id)
 SELECT 'database',id,$1,$1,$2||' / '||name,$3||' / '||name,'Перенос родительского экземпляра: '||$4,policy,policy,version,$5 FROM children`, after.ID, from, to, reason, actor)
		if err != nil {
			return importDatabaseError(err)
		}
	}
	details, _ := json.Marshal(map[string]any{"from": from, "to": to, "reason": reason, "version": after.Version})
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id,details)VALUES($1,'object_move',$2,$3,$4)`, actor, after.Kind, after.ID, details)
	return err
}
func (s *Service) buildMove(ctx context.Context, tx pgx.Tx, r MoveRequest) (MovePreview, error) {
	out := MovePreview{Conflicts: []string{}}
	if r.Kind != "instance" && r.Kind != "database" {
		return out, errors.New("Перемещать можно экземпляр или БД")
	}
	if strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 2000 {
		return out, errors.New("Укажите причину перемещения до 2000 символов")
	}
	o, err := loadObject(ctx, tx, r.Kind, r.ID, false)
	if err != nil {
		return out, err
	}
	if o.Version != r.Version {
		return out, errors.New("Объект изменился. Обновите каталог")
	}
	if o.ParentID == r.ParentID {
		return out, errors.New("Выберите другого родителя")
	}
	parentKind := "instance"
	if o.Kind == "instance" || o.DBMS == "Oracle Database" {
		parentKind = "server"
	}
	parent, err := loadObject(ctx, tx, parentKind, r.ParentID, false)
	if err != nil {
		return out, err
	}
	if parent.Version != r.ParentVersion {
		return out, errors.New("Родитель изменился. Выберите его заново")
	}
	out.FromDBMS = o.DBMS
	out.ToDBMS = o.DBMS
	if o.Kind == "database" && o.DBMS != "Oracle Database" {
		source, err := loadObject(ctx, tx, "instance", o.ParentID, false)
		if err != nil {
			return out, err
		}
		out.FromDBMS = source.DBMS
		out.ToDBMS = parent.DBMS
	}
	out.Object = o
	out.Destination = parent
	out.FromPath, err = objectPathTx(ctx, tx, o)
	if err != nil {
		return out, err
	}
	out.FromPolicy, err = effectiveForObject(ctx, tx, o)
	if err != nil {
		return out, err
	}
	after := o
	after.ParentID = parent.ID
	out.ToPath, err = objectPathTx(ctx, tx, after)
	if err != nil {
		return out, err
	}
	out.ToPolicy, err = effectiveForObject(ctx, tx, after)
	if err != nil {
		return out, err
	}
	if o.Kind == "instance" {
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM databases WHERE instance_id=$1`, o.ID).Scan(&out.Databases); err != nil {
			return out, err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM object_attachments WHERE(entity=$1 AND object_id=$2) OR($1='instance' AND entity='database' AND object_id IN(SELECT id FROM databases WHERE instance_id=$2))`, o.Kind, o.ID).Scan(&out.Attachments); err != nil {
		return out, err
	}
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return out, err
	}
	raw, _ := json.Marshal([]any{schema, s.user.ID, r, out})
	out.Fingerprint = fmt.Sprintf("%x", sha256.Sum256(raw))
	return out, nil
}
func (s *Service) PreviewMove(r MoveRequest) (MovePreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return MovePreview{}, err
	}
	defer tx.Rollback(ctx)
	p, err := s.buildMove(ctx, tx, r)
	if err != nil {
		return p, err
	}
	writer, err := s.catalogWriter(ctx, tx)
	if err != nil {
		return p, err
	}
	writer.moveReason = r.Reason
	o := p.Object
	o.ParentID = r.ParentID
	if _, err = writer.saveObject(ctx, o, writeNormal); err != nil {
		p.Conflicts = append(p.Conflicts, err.Error())
	}
	return p, nil
}
func (s *Service) ApplyMove(r MoveRequest, fingerprint string) (Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fingerprint == "" {
		return Object{}, errors.New("Проверьте предпросмотр перемещения")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if s.pool == nil {
		return Object{}, errors.New("Нет подключения")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Object{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.authorize(ctx, tx, true, false); err != nil {
		return Object{}, err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE servers,instances,databases,backup_policies IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return Object{}, err
	}
	p, err := s.buildMove(ctx, tx, r)
	if err != nil {
		return Object{}, err
	}
	if p.Fingerprint != fingerprint {
		return Object{}, errors.New("Условия перемещения изменились. Проверьте предпросмотр заново")
	}
	writer, err := s.catalogWriter(ctx, tx)
	if err != nil {
		return Object{}, err
	}
	writer.moveReason = r.Reason
	o := p.Object
	o.ParentID = r.ParentID
	saved, err := writer.saveObject(ctx, o, writeNormal)
	if err != nil {
		return Object{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Object{}, importDatabaseError(err)
	}
	return saved, nil
}
func (s *Service) GetMovements(kind, id string, page int) (MovementPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := MovementPage{Items: []Movement{}}
	if kind != "instance" && kind != "database" {
		return out, errors.New("Нет перемещений для этого типа объекта")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM object_movements WHERE entity=$1 AND object_id=$2`, kind, id).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, `SELECT m.id,m.from_path,m.to_path,m.reason,m.object_version,u.login,m.created_at,m.from_policy,m.to_policy FROM object_movements m JOIN users u ON u.id=m.actor_id WHERE m.entity=$1 AND m.object_id=$2 ORDER BY m.id DESC LIMIT 25 OFFSET $3`, kind, id, out.Page*25)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Movement
		var fp, tp []byte
		if err = rows.Scan(&m.ID, &m.FromPath, &m.ToPath, &m.Reason, &m.Version, &m.Actor, &m.CreatedAt, &fp, &tp); err != nil {
			return out, err
		}
		if len(fp) > 0 {
			m.FromPolicy = &Policy{}
			if err = json.Unmarshal(fp, m.FromPolicy); err != nil {
				return out, err
			}
		}
		if len(tp) > 0 {
			m.ToPolicy = &Policy{}
			if err = json.Unmarshal(tp, m.ToPolicy); err != nil {
				return out, err
			}
		}
		out.Items = append(out.Items, m)
	}
	return out, rows.Err()
}
