package application

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/yomezza/test/internal/security"
	"github.com/yomezza/test/internal/spreadsheet"
)

func (s *Service) ExportExcel() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exportExcelLocked()
}
func (s *Service) exportExcelLocked() (string, error) {
	return s.exportSelectionLocked(nil)
}
func (s *Service) exportSelectionLocked(ids []string) (string, error) {
	if ids != nil {
		if len(ids) == 0 || len(ids) > 100000 {
			return "", errors.New("Выберите от 1 до 100000 БД")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				return "", errors.New("Повтор ID в выборе")
			}
			seen[id] = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	query, err := tx.Query(ctx, `SELECT to_jsonb(d),to_jsonb(i),to_jsonb(s) FROM databases d LEFT JOIN instances i ON i.id=d.instance_id JOIN servers s ON s.id=coalesce(d.server_id,i.server_id) WHERE $1::uuid[] IS NULL OR d.id=ANY($1::uuid[]) ORDER BY lower(s.name),s.id,i.name,d.name,d.id`, ids)
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
		instance := Object{}
		if len(i) > 0 {
			instance, e = decodeObject(i, "instance")
		}
		if e != nil {
			query.Close()
			return "", e
		}
		server, e := decodeObject(srv, "server")
		if e != nil {
			query.Close()
			return "", e
		}
		if db.DBMS == "Oracle Database" {
			instance.Name = ""
			instance.DBMS = db.DBMS
			instance.DBMSVersion = db.DBMSVersion
		}
		a, sa := db.Attributes, server.Attributes
		rows = append(rows, [21]string{db.Name, instance.Name, db.Status, db.Environment, server.Name, instance.DBMS, instance.DBMSVersion, sa.ConnectionMethod, sa.Domain, server.Address, sa.Location, a.InformationSystem, a.ServiceOrganization, a.Supervisor, a.Responsible, server.OS, sa.OSVersion, sa.OSFamily, db.Purpose, strings.Join(a.Administration, ";"), a.PersonalData})
	}
	query.Close()
	if err = query.Err(); err != nil {
		return "", importDatabaseError(err)
	}
	if ids != nil && len(rows) != len(ids) {
		return "", errors.New("Часть выбранных БД удалена. Обновите каталог")
	}
	summary := [][2]string{{"Показатель", "Значение"}, {"Экспорт выбранных БД", strconv.Itoa(len(rows))}}
	if ids == nil {
		stats, err := loadStatistics(ctx, tx)
		if err != nil {
			return "", err
		}
		summary = [][2]string{{"Показатель", "Значение"}, {"Записи серверов", strconv.FormatInt(stats.Servers, 10)}, {"Уникальные имена серверов", strconv.FormatInt(stats.UniqueServers, 10)}, {"Пары сервер / адрес", strconv.FormatInt(stats.ServerAddresses, 10)}, {"Экземпляры СУБД", strconv.FormatInt(stats.Instances, 10)}, {"Базы данных", strconv.FormatInt(stats.Databases, 10)}, {"БД с активной эффективной политикой РК", strconv.FormatInt(stats.ProtectedDatabases, 10)}, {"БД без политики РК", strconv.FormatInt(stats.UnprotectedDatabases, 10)}, {"БД с отключённой политикой РК", strconv.FormatInt(stats.DisabledDatabases, 10)}}
		for _, d := range stats.DBMS {
			summary = append(summary, [2]string{"БД: " + d.Name, strconv.FormatInt(d.Databases, 10)})
		}

	}
	data, err := spreadsheet.WriteWithSummary(rows, summary)
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
