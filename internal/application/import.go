package application

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"

	"github.com/yomezza/test/internal/security"
	"github.com/yomezza/test/internal/spreadsheet"
)

func (s *Service) PreviewExcel(encoded string) (ImportPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingImport = nil
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return ImportPreview{}, err
	}
	defer tx.Rollback(ctx)
	if len(encoded) > ((spreadsheet.MaxFileSize+2)/3)*4 {
		return ImportPreview{}, errors.New("Файл превышает 20 МБ")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ImportPreview{}, errors.New("Некорректный файл")
	}
	wb, err := spreadsheet.Read(data)
	if err != nil {
		return ImportPreview{}, err
	}
	plan, err := s.buildImportPlan(ctx, tx, wb)
	if err != nil {
		return ImportPreview{}, err
	}
	s.pendingImport = plan
	return plan.preview, nil
}
func (s *Service) buildImportPlan(ctx context.Context, tx pgx.Tx, wb spreadsheet.Workbook) (*importPlan, error) {
	idx, err := loadIndex(ctx, tx)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	plan := &importPlan{workbook: wb, preview: ImportPreview{Token: security.ID(), Sheet: wb.Sheet, Columns: wb.Columns, Total: len(wb.Rows), Ignored: wb.IgnoredRows, SecretsIgnored: wb.SecretRows, BackupRows: wb.BackupRows, Rows: []ImportRowPreview{}}, versions: map[string]objectStamp{}, expires: time.Now().Add(10 * time.Minute), userID: s.user.ID}
	seenServers, seenInstances, seenDB := map[string]Object{}, map[string]Object{}, map[string]bool{}
	for _, row := range wb.Rows {
		item := objectsFromRow(row)
		validateImportItem(ctx, tx, &item)
		sk := serverKey(item.server.Name)
		ik := instanceKey(sk, item.instance.Name)
		dk := databaseKey(ik, item.database.Name)
		if idx.ambiguous[sk] {
			item.errors = append(item.errors, "В каталоге несколько серверов с таким именем; устраните неоднозначность")
		}
		if old, ok := seenServers[sk]; ok && !serverEquivalent(old, item.server) {
			item.errors = append(item.errors, "Повторяющиеся строки сервера содержат разные общие параметры")
		}
		if old, ok := seenInstances[ik]; ok && (old.DBMS != item.instance.DBMS || old.DBMSVersion != item.instance.DBMSVersion) {
			item.errors = append(item.errors, "Один экземпляр указан с разными типами/версиями СУБД")
		}
		if old := idx.instances[ik]; old.ID != "" && old.DBMS != item.instance.DBMS {
			item.errors = append(item.errors, "Тип существующего экземпляра не совпадает; автоматически менять его нельзя")
		}
		if seenDB[dk] {
			item.errors = append(item.errors, "Повтор SID внутри одного экземпляра в исходном файле")
		}
		if len(item.errors) == 0 {
			plan.preview.Valid++
			seenServers[sk] = item.server
			seenInstances[ik] = item.instance
			seenDB[dk] = true
			for _, key := range []string{sk, ik, dk} {
				o := idx.get(key)
				plan.versions[key] = objectStamp{o.ID, o.Version}
			}
		}
		plan.items = append(plan.items, item)
		plan.preview.Rows = append(plan.preview.Rows, ImportRowPreview{Row: item.number, Server: item.server.Name, Instance: item.instance.Name, Database: item.database.Name, Errors: item.errors, Values: append([]string{}, row.Values[:21]...)})
	}
	return plan, nil
}

// RevalidateExcel rebuilds the entire plan; a successful revision invalidates its previous token.
func (s *Service) RevalidateExcel(token string, edits []ImportRowEdit) (ImportPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.pendingImport
	if old == nil || old.preview.Token != token || time.Now().After(old.expires) || s.user == nil || old.userID != s.user.ID {
		return ImportPreview{}, errors.New("Предпросмотр устарел. Повторно выберите файл")
	}
	if len(edits) == 0 || len(edits) > len(old.workbook.Rows) {
		return ImportPreview{}, errors.New("Укажите исправляемые строки")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return ImportPreview{}, err
	}
	defer tx.Rollback(ctx)
	wb := old.workbook
	wb.Rows = append([]spreadsheet.Row{}, wb.Rows...)
	positions := map[int]int{}
	seen := map[int]bool{}
	for i, row := range wb.Rows {
		positions[row.Number] = i
	}
	for _, edit := range edits {
		i, ok := positions[edit.Row]
		if !ok || seen[edit.Row] || len(edit.Values) != 21 {
			return ImportPreview{}, errors.New("Некорректная строка исправлений: требуется 21 поле каталога")
		}
		seen[edit.Row] = true
		for c, v := range edit.Values {
			if len(v) > 20000 {
				return ImportPreview{}, errors.New("Слишком длинное поле исправления")
			}
			wb.Rows[i].Values[c] = strings.TrimSpace(v)
		}
	}
	next, err := s.buildImportPlan(ctx, tx, wb)
	if err != nil {
		return ImportPreview{}, err
	}
	s.pendingImport = next
	return next.preview, nil
}
func (s *Service) ApplyExcel(token string, skipInvalid, catalogOnly bool) (ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := ImportResult{}
	plan := s.pendingImport
	if plan == nil || plan.preview.Token != token || time.Now().After(plan.expires) || s.user == nil || plan.userID != s.user.ID {
		return result, errors.New("Предпросмотр устарел. Повторно выберите файл")
	}
	if plan.preview.Columns == 28 && !catalogOnly {
		return result, errors.New("Подтвердите импорт только каталога: политики РК и пароли архива из этого файла не переносятся")
	}
	if plan.preview.Valid == 0 {
		return result, errors.New("Нет корректных строк для импорта")
	}
	if plan.preview.Valid != plan.preview.Total && !skipInvalid {
		return result, errors.New("В файле есть ошибки. Исправьте файл или явно разрешите пропуск ошибочных строк")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	// READ COMMITTED takes a fresh snapshot after waiting for table locks.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, importDatabaseError(err)
	}
	defer tx.Rollback(ctx)
	if err = s.authorize(ctx, tx, true, false); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE servers,instances,databases IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return result, importDatabaseError(err)
	}
	idx, err := loadIndex(ctx, tx)
	if err != nil {
		return result, importDatabaseError(err)
	}
	if err = ensureUnchanged(idx, plan.versions); err != nil {
		return result, err
	}
	for _, item := range plan.items {
		if len(item.errors) > 0 {
			result.Skipped++
			continue
		}
		sk := serverKey(item.server.Name)
		server, err := s.saveImportedObject(ctx, tx, idx.servers[sk], item.server, &result)
		if err != nil {
			return ImportResult{}, err
		}
		idx.servers[sk] = server
		item.instance.ParentID = server.ID
		ik := instanceKey(sk, item.instance.Name)
		instance, err := s.saveImportedObject(ctx, tx, idx.instances[ik], item.instance, &result)
		if err != nil {
			return ImportResult{}, err
		}
		idx.instances[ik] = instance
		item.database.ParentID = instance.ID
		dk := databaseKey(ik, item.database.Name)
		db, err := s.saveImportedObject(ctx, tx, idx.databases[dk], item.database, &result)
		if err != nil {
			return ImportResult{}, err
		}
		idx.databases[dk] = db
	}
	if err = event(ctx, tx, s.user.ID, "excel_import", "import", security.ID()); err != nil {
		return ImportResult{}, importDatabaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return ImportResult{}, importDatabaseError(err)
	}
	s.pendingImport = nil
	return result, nil
}
