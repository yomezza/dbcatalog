package application

import (
	"context"
	"errors"
	"time"
)

// SetImportRowsExcluded revises a pending plan; the source workbook and live catalog are untouched.
func (s *Service) SetImportRowsExcluded(token string, rows []int, exclude bool) (ImportPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.pendingImport
	if old == nil || old.preview.Token != token || time.Now().After(old.expires) || s.user == nil || old.userID != s.user.ID {
		return ImportPreview{}, errors.New("Предпросмотр устарел. Повторно выберите файл")
	}
	if len(rows) == 0 || len(rows) > len(old.workbook.Rows) {
		return ImportPreview{}, errors.New("Выберите строки предпросмотра")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return ImportPreview{}, err
	}
	defer tx.Rollback(ctx)
	available := map[int]bool{}
	for _, r := range old.workbook.Rows {
		available[r.Number] = true
	}
	nextExcluded := map[int]bool{}
	for id, v := range old.excluded {
		nextExcluded[id] = v
	}
	seen := map[int]bool{}
	for _, id := range rows {
		if !available[id] || seen[id] {
			return ImportPreview{}, errors.New("Неизвестная или повторная строка предпросмотра")
		}
		seen[id] = true
		if exclude {
			nextExcluded[id] = true
		} else {
			delete(nextExcluded, id)
		}
	}
	next, err := s.buildImportPlanExcluded(ctx, tx, old.workbook, nextExcluded)
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
