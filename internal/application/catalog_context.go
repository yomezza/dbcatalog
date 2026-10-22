package application

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type ObjectContext struct {
	Object      Object         `json:"object"`
	Catalog     Catalog        `json:"catalog"`
	Policies    []Policy       `json:"policies"`
	ServerPage  int            `json:"serverPage"`
	BranchPages map[string]int `json:"branchPages"`
	Path        string         `json:"path"`
}

func loadObject(ctx context.Context, tx pgx.Tx, kind, id string, lock bool) (Object, error) {
	t, err := table(kind)
	if err != nil {
		return Object{}, err
	}
	sql := "SELECT to_jsonb(t)-'search_text' FROM " + t + " t WHERE id=$1"
	if lock {
		sql += " FOR KEY SHARE"
	}
	var raw []byte
	if err = tx.QueryRow(ctx, sql, id).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Object{}, errors.New("Объект не найден")
		}
		return Object{}, importDatabaseError(err)
	}
	return decodeObject(raw, kind)
}
func loadObjectContext(ctx context.Context, tx pgx.Tx, kind, id string) (ObjectContext, error) {
	out := ObjectContext{Catalog: Catalog{Servers: []Object{}, Instances: []Object{}, Databases: []Object{}}, Policies: []Policy{}, BranchPages: map[string]int{}}
	var err error
	out.Object, err = loadObject(ctx, tx, kind, id, false)
	if err != nil {
		return out, err
	}
	o := out.Object
	var server, instance Object
	switch kind {
	case "server":
		server = o
	case "instance":
		instance = o
		server, err = loadObject(ctx, tx, "server", o.ParentID, false)
	case "database":
		out.Catalog.Databases = append(out.Catalog.Databases, o)
		if o.DBMS == "Oracle Database" {
			server, err = loadObject(ctx, tx, "server", o.ParentID, false)
		} else {
			instance, err = loadObject(ctx, tx, "instance", o.ParentID, false)
			if err == nil {
				server, err = loadObject(ctx, tx, "server", instance.ParentID, false)
			}
		}
	}
	if err != nil {
		return out, err
	}
	out.Catalog.Servers = append(out.Catalog.Servers, server)
	out.Path = server.Name
	if instance.ID != "" {
		out.Catalog.Instances = append(out.Catalog.Instances, instance)
		out.Path += " / " + instance.Name
	}
	if kind == "database" {
		if o.DBMS == "Oracle Database" {
			out.Path += " / Oracle Database"
		}
		out.Path += " / " + o.Name
	}
	if err = tx.QueryRow(ctx, `SELECT count(*)/10 FROM servers WHERE (lower(name),id)<(lower($1),$2::uuid)`, server.Name, server.ID).Scan(&out.ServerPage); err != nil {
		return out, err
	}
	if instance.ID != "" {
		var page int
		if err = tx.QueryRow(ctx, `SELECT count(*)/100 FROM instances WHERE server_id=$1 AND(lower(name),id)<(lower($2),$3::uuid)`, server.ID, instance.Name, instance.ID).Scan(&page); err != nil {
			return out, err
		}
		out.BranchPages[server.ID] = page
	}
	if kind == "database" {
		var page int
		column, key := "instance_id", o.ParentID
		if o.DBMS == "Oracle Database" {
			column, key = "server_id", "oracle:"+o.ParentID
		}
		if err = tx.QueryRow(ctx, "SELECT count(*)/100 FROM databases WHERE "+column+"=$1 AND(lower(name),id)<(lower($2),$3::uuid)", o.ParentID, o.Name, o.ID).Scan(&page); err != nil {
			return out, err
		}
		out.BranchPages[key] = page
	}
	rows, err := tx.Query(ctx, `SELECT to_jsonb(p) FROM backup_policies p WHERE instance_id=$1 OR database_id=$2 ORDER BY id`, nullableID(instance.ID), nullableID(func() string {
		if kind == "database" {
			return o.ID
		}
		return ""
	}()))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		p, e := decodePolicy(raw)
		if e != nil {
			return out, e
		}
		out.Policies = append(out.Policies, p)
	}
	return out, rows.Err()
}
func (s *Service) GetObjectContext(kind, id string) (ObjectContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return ObjectContext{}, err
	}
	defer tx.Rollback(ctx)
	return loadObjectContext(ctx, tx, kind, id)
}

type ObjectOptions struct {
	Items []ObjectOption `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
}
type ObjectOption struct {
	Object Object `json:"object"`
	Path   string `json:"path"`
}

func (s *Service) FindObjects(kind, query string, page int) (ObjectOptions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ObjectOptions{Items: []ObjectOption{}}
	t, err := table(kind)
	if err != nil {
		return out, err
	}
	if len(query) > 2000 {
		return out, errors.New("Слишком длинный запрос")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	joins, path, eligible := "", "o.name", "eligible_server"
	if kind == "instance" {
		joins = " JOIN servers s ON s.id=o.server_id"
		path = "s.name||' / '||o.name"
		eligible = "eligible_instance"
	} else if kind == "database" {
		joins = " LEFT JOIN instances i ON i.id=o.instance_id JOIN servers s ON s.id=coalesce(o.server_id,i.server_id)"
		path = "concat_ws(' / ',s.name,coalesce(i.name,'Oracle Database'),o.name)"
		eligible = "eligible_db"
	}
	args, err := catalogMatchArgs(CatalogCriteria{Query: query})
	if err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, catalogMatchCTE+"SELECT count(*) FROM "+eligible, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Page = boundedPage(page, out.Total, 25)
	args = append(args, out.Page*25)
	rows, err := tx.Query(ctx, catalogMatchCTE+"SELECT to_jsonb(o)-'search_text',"+path+" FROM "+t+" o JOIN "+eligible+" eligible ON eligible.id=o.id"+joins+" ORDER BY lower(o.name),o.id LIMIT 25 OFFSET $8", args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var item ObjectOption
		if err = rows.Scan(&raw, &item.Path); err != nil {
			return out, err
		}
		item.Object, err = decodeObject(raw, kind)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
