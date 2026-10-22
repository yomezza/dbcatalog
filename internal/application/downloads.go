package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/yomezza/test/internal/downloads"
	"github.com/yomezza/test/internal/spreadsheet"
	"path/filepath"
	"time"
)

func (s *Service) downloadStore(ctx context.Context) (downloads.Store, error) {
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return downloads.Store{}, err
	}
	defer tx.Rollback(ctx)
	scope := fmt.Sprintf("%x", sha256.Sum256([]byte(s.connectionKey+":"+s.user.ID)))
	return downloads.Store{Root: filepath.Join(filepath.Dir(s.path), "downloads", scope)}, nil
}
func (s *Service) ListDownloads() ([]downloads.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := s.downloadStore(ctx)
	if err != nil {
		return nil, err
	}
	return store.List()
}
func (s *Service) ResolveDownload(id string) (downloads.Entry, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := s.downloadStore(ctx)
	if err != nil {
		return downloads.Entry{}, "", err
	}
	return store.Resolve(id)
}
func (s *Service) ExportToDownloads() (downloads.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := s.downloadStore(ctx)
	if err != nil {
		return downloads.Entry{}, err
	}
	encoded, err := s.exportExcelLocked()
	if err != nil {
		return downloads.Entry{}, err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return downloads.Entry{}, err
	}
	e, err := store.Add("CatalogDB-"+time.Now().Format("2006-01-02-150405")+".xlsx", "Экспорт каталога", data)
	if err != nil {
		return downloads.Entry{}, errors.New("Не удалось сохранить экспорт в локальные загрузки")
	}
	return e, nil
}
func (s *Service) ExportImportPreview(token string) (downloads.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := s.downloadStore(ctx)
	if err != nil {
		return downloads.Entry{}, err
	}
	plan := s.pendingImport
	if plan == nil || plan.preview.Token != token || time.Now().After(plan.expires) || plan.userID != s.user.ID {
		return downloads.Entry{}, errors.New("Предпросмотр устарел")
	}
	rows := [][21]string{}
	for _, row := range plan.workbook.Rows {
		var values [21]string
		copy(values[:], row.Values[:21])
		rows = append(rows, values)
	}
	data, err := spreadsheet.Write(rows)
	if err != nil {
		return downloads.Entry{}, err
	}
	e, err := store.Add("CatalogDB-исправления-"+time.Now().Format("2006-01-02-150405")+".xlsx", "Предпросмотр импорта (21 колонка)", data)
	if err != nil {
		return downloads.Entry{}, errors.New("Не удалось сохранить исправленный файл")
	}
	return e, nil
}
