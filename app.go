package main

import (
	"github.com/yomezza/test/internal/application"
	"github.com/yomezza/test/internal/config"
	"os"
	"slices"
)

type App struct{ service *application.Service }

func NewApp() (*App, error) {
	path, err := config.Path()
	if err != nil {
		return nil, err
	}
	return &App{service: application.New(path, application.OSStore{})}, nil
}
func (a *App) GetConnection() (application.SavedConnection, error) { return a.service.GetConnection() }
func (a *App) Connect(c config.Connection, password string, useSaved, remember bool) (application.Status, error) {
	return a.service.Connect(c, password, useSaved, remember)
}
func (a *App) GetStatus() (application.Status, error) { return a.service.GetStatus() }
func (a *App) Register(login, password string, bootstrap bool) error {
	return a.service.Register(login, password, bootstrap)
}
func (a *App) Login(login, password string) (application.User, error) {
	return a.service.Login(login, password)
}
func (a *App) Logout()                                  { a.service.Logout() }
func (a *App) GetCatalog() (application.Catalog, error) { return a.service.GetCatalog() }
func (a *App) SaveObject(o application.Object) (application.Object, error) {
	return a.service.SaveObject(o)
}
func (a *App) DeleteObject(kind, id string, version int64) error {
	return a.service.DeleteObject(kind, id, version)
}
func (a *App) ListUsers() ([]application.User, error)  { return a.service.ListUsers() }
func (a *App) UpdateUser(id, role, state string) error { return a.service.UpdateUser(id, role, state) }
func (a *App) GetHistory(kind, id string) ([]application.HistoryEntry, error) {
	return a.service.GetHistory(kind, id)
}

func (a *App) IsSetupMode() bool { return slices.Contains(os.Args[1:], "--setup-connection") }

func (a *App) GetLookups() (map[string][]string, error)     { return a.service.GetLookups() }
func (a *App) ListFavorites() ([]application.Object, error) { return a.service.ListFavorites() }
func (a *App) SetFavorite(kind, id string, enabled bool) error {
	return a.service.SetFavorite(kind, id, enabled)
}

func (a *App) PreviewExcel(encoded string) (application.ImportPreview, error) {
	return a.service.PreviewExcel(encoded)
}
func (a *App) ApplyExcel(token string, skipInvalid, catalogOnly bool) (application.ImportResult, error) {
	return a.service.ApplyExcel(token, skipInvalid, catalogOnly)
}
func (a *App) ExportExcel() (string, error) { return a.service.ExportExcel() }

func (a *App) ListPolicies() ([]application.Policy, error) { return a.service.ListPolicies() }
func (a *App) SavePolicy(p application.Policy) (application.Policy, error) {
	return a.service.SavePolicy(p)
}
func (a *App) DeletePolicy(id string, version int64) error {
	return a.service.DeletePolicy(id, version)
}
func (a *App) GetEffectivePolicy(id string) (application.EffectivePolicy, error) {
	return a.service.GetEffectivePolicy(id)
}

func (a *App) GetPolicyHistory(id string) ([]application.HistoryEntry, error) {
	return a.service.GetHistory("policy", id)
}
