package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/buildinfo"
	"github.com/yomezza/test/internal/downloads"
	"github.com/yomezza/test/internal/reports"
)

// ExportObjectPDF reads the object, parents, policy and counts from one
// authorized snapshot. No client-provided record contents enter the report.
func (s *Service) ExportObjectPDF(kind, id string) (downloads.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := s.downloadStore(ctx)
	if err != nil {
		return downloads.Entry{}, err
	}
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return downloads.Entry{}, err
	}
	defer tx.Rollback(ctx)
	now := time.Now()
	report, err := buildObjectPassport(ctx, tx, kind, id, s.user.Login, now)
	if err != nil {
		return downloads.Entry{}, err
	}
	data, err := reports.RenderPassport(report)
	if err != nil {
		return downloads.Entry{}, errors.New("Не удалось сформировать PDF-паспорт объекта")
	}
	if err = event(ctx, tx, s.user.ID, "pdf_export", kind, id); err != nil {
		return downloads.Entry{}, importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return downloads.Entry{}, importDatabaseError(err)
	}
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, report.Name)
	name = string([]rune(name)[:min(60, len([]rune(name)))])
	name = "CatalogDB-Паспорт-" + strings.Trim(name, " .") + "-" + now.Format("2006-01-02-150405") + ".pdf"
	entry, err := store.Add(name, "PDF-паспорт объекта", data)
	if err != nil {
		return downloads.Entry{}, errors.New("Не удалось сохранить PDF в локальные загрузки")
	}
	return entry, nil
}

func buildObjectPassport(ctx context.Context, tx pgx.Tx, kind, id, author string, now time.Time) (reports.Passport, error) {
	c, err := loadObjectContext(ctx, tx, kind, id)
	if err != nil {
		return reports.Passport{}, err
	}
	o := c.Object
	server := c.Catalog.Servers[0]
	var instance Object
	if len(c.Catalog.Instances) > 0 {
		instance = c.Catalog.Instances[0]
	}
	dbms, dbmsVersion, port := o.DBMS, o.DBMSVersion, o.Port
	if kind == "database" && o.DBMS != "Oracle Database" {
		dbms, dbmsVersion, port = instance.DBMS, instance.DBMSVersion, instance.Port
	}
	labels := map[string]string{"server": "Сервер", "instance": "Экземпляр СУБД", "database": "База данных"}
	out := reports.Passport{Kind: labels[kind], Name: o.Name, Path: c.Path, ID: o.ID, Version: o.Version, Author: author, GeneratedAt: now, AppVersion: buildinfo.Version, Badges: []string{o.Environment, o.Status, dbms}}
	add := func(title string, fields ...reports.Field) {
		out.Sections = append(out.Sections, reports.Section{Title: title, Fields: fields})
	}
	if kind == "server" {
		var instances, databases, oracle int64
		err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM instances WHERE server_id=$1),
		 (SELECT count(*) FROM databases d LEFT JOIN instances i ON i.id=d.instance_id WHERE coalesce(d.server_id,i.server_id)=$1),
		 (SELECT count(*) FROM databases WHERE server_id=$1)`, id).Scan(&instances, &databases, &oracle)
		if err != nil {
			return out, importDatabaseError(err)
		}
		out.Summary = []reports.Field{{Label: "Экземпляры СУБД", Value: fmt.Sprint(instances)}, {Label: "Базы данных", Value: fmt.Sprint(databases)}, {Label: "Из них Oracle SID", Value: fmt.Sprint(oracle)}}
	} else if kind == "instance" {
		var count int64
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM databases WHERE instance_id=$1`, id).Scan(&count); err != nil {
			return out, importDatabaseError(err)
		}
		out.Summary = []reports.Field{{Label: "СУБД", Value: dbms}, {Label: "Версия СУБД", Value: dbmsVersion}, {Label: "Базы данных", Value: fmt.Sprint(count)}}
	} else {
		policyLabel := "Не назначена"
		for _, p := range c.Policies {
			if p.ObjectID == o.ID || p.Kind == "instance" {
				policyLabel = "Включена"
				if !p.Enabled {
					policyLabel = "Отключена"
				}
			}
			if p.ObjectID == o.ID {
				break
			}
		}
		out.Summary = []reports.Field{{Label: "СУБД", Value: dbms}, {Label: "Сервер", Value: server.Name}, {Label: "Политика РК", Value: policyLabel}}
	}
	main := []reports.Field{{Label: "Имя объекта", Value: o.Name}, {Label: "Состояние", Value: o.Status}, {Label: "Тип эксплуатации", Value: o.Environment}}
	if kind != "server" {
		main = append(main, reports.Field{Label: "Тип СУБД", Value: dbms}, reports.Field{Label: "Версия СУБД", Value: dbmsVersion})
		portText := ""
		if port != nil {
			portText = fmt.Sprint(*port)
		}
		main = append(main, reports.Field{Label: "Порт СУБД", Value: portText})
	}
	if kind == "database" {
		main = append(main, reports.Field{Label: "Информационная система", Value: o.Attributes.InformationSystem}, reports.Field{Label: "Назначение", Value: o.Purpose}, reports.Field{Label: "Персональные данные", Value: o.Attributes.PersonalData})
	}
	add("Основные сведения", main...)
	placement := []reports.Field{{Label: "Сервер", Value: server.Name}, {Label: "Адрес сервера", Value: server.Address}, {Label: "Месторасположение", Value: server.Attributes.Location}}
	if instance.ID != "" && kind == "database" {
		placement = append(placement, reports.Field{Label: "Экземпляр СУБД", Value: instance.Name})
	}
	if o.DBMS == "Oracle Database" {
		placement = append(placement, reports.Field{Label: "Размещение SID", Value: "Непосредственно на сервере; отдельный экземпляр не требуется"})
	}
	placement = append(placement, reports.Field{Label: "Подключение к серверу", Value: server.Attributes.ConnectionMethod}, reports.Field{Label: "Доменное членство", Value: server.Attributes.Domain}, reports.Field{Label: "Операционная система", Value: server.OS}, reports.Field{Label: "Версия ОС", Value: server.Attributes.OSVersion}, reports.Field{Label: "Семейство ОС", Value: server.Attributes.OSFamily})
	add("Размещение и техническая среда", placement...)
	add("Ответственные и сопровождение", reports.Field{Label: "Обслуживающая организация", Value: o.Attributes.ServiceOrganization}, reports.Field{Label: "Сопровождение (руководитель)", Value: o.Attributes.Supervisor}, reports.Field{Label: "Ответственное лицо", Value: o.Attributes.Responsible}, reports.Field{Label: "Работа АБД", Value: strings.Join(o.Attributes.Administration, "\n")})
	if kind != "server" {
		var policy *Policy
		for i := range c.Policies {
			p := &c.Policies[i]
			if p.ObjectID == o.ID {
				policy = p
				break
			}
			if kind == "database" && p.Kind == "instance" {
				policy = p
			}
		}
		fields := []reports.Field{{Label: "Политика", Value: "Не назначена"}}
		if policy != nil {
			state := "Включена"
			if !policy.Enabled {
				state = "Отключена"
			}
			source := "Назначена объекту"
			if kind == "database" && policy.Kind == "instance" {
				source = "Унаследована от экземпляра «" + instance.Name + "»"
			}
			fields = []reports.Field{{Label: "Политика", Value: policy.Name}, {Label: "Состояние политики", Value: state}, {Label: "Источник", Value: source}, {Label: "Тип РК", Value: policy.BackupType}, {Label: "Планировщик", Value: policy.Scheduler}, {Label: "Расписание", Value: policy.Schedule}, {Label: "Путь хранения РК", Value: policy.Storage}, {Label: "Срок хранения", Value: fmt.Sprintf("%d дней", policy.RetentionDays)}}
			if policy.Description != "" {
				fields = append(fields, reports.Field{Label: "Описание политики", Value: policy.Description})
			}
		}
		fields = append(fields, reports.Field{Label: "Примечание", Value: "Политика описывает настройки резервного копирования. Сведения о фактическом выполнении и успешности копий в этот паспорт не входят."})
		add("Резервное копирование", fields...)
	}
	if strings.TrimSpace(o.Description) != "" {
		add("Описание объекта", reports.Field{Label: "Описание", Value: o.Description})
	}
	return out, nil
}
