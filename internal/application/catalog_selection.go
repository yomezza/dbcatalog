package application

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/yomezza/test/internal/downloads"
	"time"
)

func (s *Service) SelectCatalogResult(f CatalogCriteria) ([]ObjectRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	args, err := catalogMatchArgs(f)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, catalogMatchCTE+`SELECT d.id::text,d.version FROM databases d JOIN eligible_db e ON e.id=d.id ORDER BY d.id LIMIT 10001`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ObjectRef{}
	for rows.Next() {
		var ref ObjectRef
		if err = rows.Scan(&ref.ID, &ref.Version); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 10000 {
		return nil, errors.New("Результат содержит более 10000 БД. Уточните фильтры для массового выбора")
	}
	return out, nil
}
func (s *Service) CountSelectionOutside(f CatalogCriteria, ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) > 10000 {
		return 0, errors.New("Слишком большой выбор")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	args, err := catalogMatchArgs(f)
	if err != nil {
		return 0, err
	}
	args = append(args, ids)
	var inside int
	if err = tx.QueryRow(ctx, catalogMatchCTE+`SELECT count(*) FROM eligible_db WHERE id=ANY($8::uuid[])`, args...).Scan(&inside); err != nil {
		return 0, err
	}
	return len(ids) - inside, nil
}
func (s *Service) ExportCatalogResult(f CatalogCriteria) (downloads.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return downloads.Entry{}, err
	}
	defer tx.Rollback(ctx)
	args, err := catalogMatchArgs(f)
	if err != nil {
		return downloads.Entry{}, err
	}
	rows, err := tx.Query(ctx, catalogMatchCTE+`SELECT id::text FROM eligible_db ORDER BY id LIMIT 100001`, args...)
	if err != nil {
		return downloads.Entry{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return downloads.Entry{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return downloads.Entry{}, err
	}
	if len(ids) == 0 || len(ids) > 100000 {
		return downloads.Entry{}, errors.New("Для экспорта выберите результат от 1 до 100000 БД")
	}
	// Keep selection resolution and data export in the same repeatable-read transaction.
	encoded, err := writeCatalogExport(ctx, tx, ids, s.user.ID)
	if err != nil {
		return downloads.Entry{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return downloads.Entry{}, importDatabaseError(err)
	}
	store, err := s.downloadStore(ctx)
	if err != nil {
		return downloads.Entry{}, err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return downloads.Entry{}, err
	}
	return store.Add("CatalogDB-result-"+time.Now().Format("2006-01-02-150405")+".xlsx", "Экспорт результата поиска и фильтров", data)
}
