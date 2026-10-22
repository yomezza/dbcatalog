package application

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/yomezza/test/internal/security"
	"github.com/yomezza/test/internal/spreadsheet"
)

func (s *Service) ExportExcel() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	query, err := tx.Query(ctx, `SELECT to_jsonb(d),to_jsonb(i),to_jsonb(s) FROM databases d JOIN instances i ON i.id=d.instance_id JOIN servers s ON s.id=i.server_id ORDER BY lower(s.name),s.id,i.name,d.name,d.id`)
	if err != nil {
		return "", importDatabaseError(err)
	}
	rows := [][21]string{}
	for query.Next() {
		var d, i, srv []byte
		if err = query.Scan(&d, &i, &srv); err != nil {
			query.Close()
			return "", importDatabaseError(err)
		}
		db, e := decodeObject(d, "database")
		if e != nil {
			query.Close()
			return "", e
		}
		instance, e := decodeObject(i, "instance")
		if e != nil {
			query.Close()
			return "", e
		}
		server, e := decodeObject(srv, "server")
		if e != nil {
			query.Close()
			return "", e
		}
		a, sa := db.Attributes, server.Attributes
		rows = append(rows, [21]string{db.Name, instance.Name, db.Status, db.Environment, server.Name, instance.DBMS, instance.DBMSVersion, sa.ConnectionMethod, sa.Domain, server.Address, sa.Location, a.InformationSystem, a.ServiceOrganization, a.Supervisor, a.Responsible, server.OS, sa.OSVersion, sa.OSFamily, db.Purpose, strings.Join(a.Administration, ";"), a.PersonalData})
	}
	query.Close()
	if err = query.Err(); err != nil {
		return "", importDatabaseError(err)
	}
	data, err := spreadsheet.Write(rows)
	if err != nil {
		return "", err
	}
	if err = event(ctx, tx, s.user.ID, "excel_export", "export", security.ID()); err != nil {
		return "", importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", importDatabaseError(err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
