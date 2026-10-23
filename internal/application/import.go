package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	return s.buildImportPlanExcluded(ctx, tx, wb, map[int]bool{})
}
func (s *Service) buildImportPlanExcluded(ctx context.Context, tx pgx.Tx, wb spreadsheet.Workbook, excluded map[int]bool) (*importPlan, error) {
	idx, err := loadIndex(ctx, tx, wb)
	if err != nil {
		return nil, importDatabaseError(err)
	}
	choices, err := loadChoices(ctx, tx)
	if err != nil {
		return nil, err
	}
	plan := &importPlan{excluded: excluded, workbook: wb, preview: ImportPreview{OriginalTotal: len(wb.Rows), ExcludedRows: []ImportRowPreview{}, Token: security.ID(), Sheet: wb.Sheet, Columns: wb.Columns, Total: 0, Ignored: wb.IgnoredRows, SecretsIgnored: 0, BackupRows: 0, Rows: []ImportRowPreview{}}, versions: map[string]objectStamp{}, expires: time.Now().Add(10 * time.Minute), userID: s.user.ID}
	seenServers, seenInstances, seenDB := map[string]Object{}, map[string]Object{}, map[string]bool{}
	firstServerRow := map[string]int{}
	for _, row := range wb.Rows {
		if excluded[row.Number] {
			plan.preview.Excluded++
			plan.preview.ExcludedRows = append(plan.preview.ExcludedRows, ImportRowPreview{Row: row.Number, Server: row.Values[4], Instance: row.Values[1], Database: row.Values[0], Errors: []string{}, Values: append([]string{}, row.Values[:21]...)})
			continue
		}
		plan.preview.Total++
		if row.HasSecrets {
			plan.preview.SecretsIgnored++
		}
		if wb.Columns == 28 {
			for _, v := range row.Values[21:26] {
				if v != "" {
					plan.preview.BackupRows++
					break
				}
			}
		}
		item := objectsFromRow(row)
		validateImportItem(choices, &item)
		sk, ik, dk := item.keys()
		if idx.ambiguous[sk] {
			item.errors = append(item.errors, "В каталоге несколько серверов с таким именем; устраните неоднозначность")
		}
		if old, ok := seenServers[sk]; ok && !serverEquivalent(old, item.server) {
			item.errors = append(item.errors, fmt.Sprintf("Повторяющиеся строки сервера «%s» содержат разные общие параметры (строки %d и %d): %s", item.server.Name, firstServerRow[sk], row.Number, strings.Join(serverDifferences(old, item.server), ", ")))
		}
		if old, ok := seenInstances[ik]; ok && (old.DBMS != item.instance.DBMS || old.DBMSVersion != item.instance.DBMSVersion) {
			item.errors = append(item.errors, "Один экземпляр указан с разными типами/версиями СУБД")
		}
		if old := idx.instances[ik]; !item.isOracle() && old.ID != "" && old.DBMS != item.instance.DBMS {
			item.errors = append(item.errors, "Тип существующего экземпляра не совпадает; автоматически менять его нельзя")
		}
		if seenDB[dk] {
			scope := "экземпляра"
			if item.isOracle() {
				scope = "сервера Oracle"
			}
			item.errors = append(item.errors, "Повтор SID внутри одного "+scope+" в исходном файле")
		}
		if len(item.errors) == 0 {
			plan.preview.Valid++
			if _, ok := seenServers[sk]; !ok {
				firstServerRow[sk] = row.Number
			}
			seenServers[sk] = item.server
			if !item.isOracle() {
				seenInstances[ik] = item.instance
			}
			seenDB[dk] = true
			for _, key := range []string{sk, ik, dk} {
				if key == "" {
					continue
				}
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
		if !ok || seen[edit.Row] || old.excluded[edit.Row] || len(edit.Values) != 21 {
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
	next, err := s.buildImportPlanExcluded(ctx, tx, wb, old.excluded)
	if err != nil {
		return ImportPreview{}, err
	}
	if old.policyConfig != nil {
		if err = s.attachImportPolicies(ctx, tx, next, *old.policyConfig); err != nil {
			return ImportPreview{}, err
		}
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
	result.Excluded = plan.preview.Excluded
	if plan.preview.WithPolicies && catalogOnly {
		return result, errors.New("Предпросмотр содержит политики: подтвердите их импорт или отключите перенос политик и повторите проверку")
	}
	if plan.preview.Columns == 28 && !catalogOnly && !plan.preview.WithPolicies {
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
	if _, err = tx.Exec(ctx, `LOCK TABLE servers,instances,databases,backup_policies IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return result, importDatabaseError(err)
	}
	idx, err := loadIndex(ctx, tx, plan.workbook)
	if err != nil {
		return result, importDatabaseError(err)
	}
	if err = ensureUnchanged(idx, plan.versions); err != nil {
		return result, err
	}
	if plan.policyConfig != nil {
		stamp, e := importPolicyStamp(ctx, tx)
		if e != nil {
			return result, e
		}
		if stamp != plan.policyStamp {
			return result, errors.New("Политики РК изменились. Повторите предпросмотр")
		}
	}
	ops := []stagedWrite{}
	importedRows := map[int]bool{}
	for _, item := range plan.items {
		if len(item.errors) > 0 {
			result.Skipped++
			continue
		}
		sk, ik, dk := item.keys()
		server, err := prepareImportedObject(idx.servers[sk], item.server, &result, &ops)
		if err != nil {
			return ImportResult{}, err
		}
		idx.servers[sk] = server
		if item.isOracle() {
			item.database.ParentID = server.ID
		} else {
			item.instance.ParentID = server.ID
			instance, err := prepareImportedObject(idx.instances[ik], item.instance, &result, &ops)
			if err != nil {
				return ImportResult{}, err
			}
			idx.instances[ik] = instance
			item.database.ParentID = instance.ID
		}
		db, err := prepareImportedObject(idx.databases[dk], item.database, &result, &ops)
		if err != nil {
			return ImportResult{}, err
		}
		idx.databases[dk] = db
		importedRows[item.number] = true
	}
	for _, planned := range plan.policies {
		applicable := false
		for _, row := range planned.rows {
			if importedRows[row] {
				applicable = true
			}
		}
		if !applicable {
			continue
		}
		p := planned.policy
		target := idx.get(planned.key)
		p.ObjectID = target.ID
		if p.ObjectID == "" {
			return ImportResult{}, errors.New("Объект назначения политики не найден")
		}
		raw, e := json.Marshal(p)
		if e != nil {
			return ImportResult{}, e
		}
		ops = append(ops, stagedWrite{"policy", raw, p.Version, 0})
		if p.ID == "" {
			result.PoliciesCreated++
		} else {
			result.PoliciesUpdated++
		}
	}
	if err = applyStagedWrites(ctx, tx, ops); err != nil {
		return ImportResult{}, err
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
