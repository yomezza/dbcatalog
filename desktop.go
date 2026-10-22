package main

import (
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yomezza/test/internal/application"
	"github.com/yomezza/test/internal/downloads"
	"os"
	"path/filepath"
)

func (a *App) ListDownloads() ([]downloads.Entry, error)   { return a.service.ListDownloads() }
func (a *App) ExportToDownloads() (downloads.Entry, error) { return a.service.ExportToDownloads() }
func (a *App) ExportImportPreview(token string) (downloads.Entry, error) {
	return a.service.ExportImportPreview(token)
}
func (a *App) OpenDownload(id string) error {
	_, path, err := a.service.ResolveDownload(id)
	if err != nil {
		return err
	}
	return openLocalFile(path, false)
}
func (a *App) ShowDownload(id string) error {
	_, path, err := a.service.ResolveDownload(id)
	if err != nil {
		return err
	}
	return openLocalFile(path, true)
}
func (a *App) SaveDownloadAs(id string) (bool, error) {
	e, path, err := a.service.ResolveDownload(id)
	if err != nil {
		return false, err
	}
	if a.ctx == nil {
		return false, errors.New("Окно приложения недоступно")
	}
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Сохранить файл", DefaultFilename: e.Name, Filters: []runtime.FileFilter{{DisplayName: "Excel (*.xlsx)", Pattern: "*.xlsx"}}})
	if err != nil {
		return false, errors.New("Не удалось открыть окно сохранения")
	}
	if target == "" {
		return false, nil
	}
	if filepath.Ext(target) == "" {
		target += ".xlsx"
	}
	if filepath.Ext(target) != ".xlsx" {
		return false, errors.New("Выберите расширение .xlsx")
	}
	if filepath.Clean(target) == filepath.Clean(path) {
		return true, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, errors.New("Не удалось прочитать сохранённый файл")
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".catalogdb-*.tmp")
	if err != nil {
		return false, errors.New("Нет доступа к выбранному каталогу")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp, target)
	}
	if err != nil {
		return false, errors.New("Не удалось сохранить файл по выбранному пути")
	}
	return true, nil
}
func (a *App) ConnectToServer(kind, id string) error {
	target, err := a.service.GetServerConnection(kind, id)
	if err != nil {
		return err
	}
	return startServerConnection(target)
}
func (a *App) RevalidateExcel(token string, edits []application.ImportRowEdit) (application.ImportPreview, error) {
	return a.service.RevalidateExcel(token, edits)
}
