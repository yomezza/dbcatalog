package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yomezza/test/internal/application"
)

func (a *App) GetDiagnostics() application.Diagnostics { return a.service.GetDiagnostics() }
func (a *App) SaveDiagnosticsReport() (bool, error) {
	if a.ctx == nil {
		return false, errors.New("Окно приложения недоступно")
	}
	data, err := json.MarshalIndent(a.GetDiagnostics(), "", "  ")
	if err != nil {
		return false, errors.New("Не удалось подготовить отчёт")
	}
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Сохранить диагностику", DefaultFilename: "CatalogDB-diagnostics.json", Filters: []runtime.FileFilter{{DisplayName: "Диагностический отчёт (*.json)", Pattern: "*.json"}}})
	if err != nil {
		return false, errors.New("Не удалось открыть окно сохранения")
	}
	if target == "" {
		return false, nil
	}
	if filepath.Ext(target) == "" {
		target += ".json"
	}
	if !strings.EqualFold(filepath.Ext(target), ".json") {
		return false, errors.New("Выберите расширение .json")
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".catalogdb-diagnostics-*.tmp")
	if err != nil {
		return false, errors.New("Нет доступа к выбранной папке")
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(data, '\n'))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), target)
	}
	if err != nil {
		return false, errors.New("Не удалось сохранить диагностический отчёт")
	}
	return true, nil
}
