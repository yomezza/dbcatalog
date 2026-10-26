package main

import (
	"context"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yomezza/test/internal/application"
	"github.com/yomezza/test/internal/config"
	"github.com/yomezza/test/internal/documents"
	"github.com/yomezza/test/internal/downloads"
	"os"
	"slices"
)

type App struct {
	service *application.Service
	ctx     context.Context
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.service.SetChangeHandler(func(change application.Change) { runtime.EventsEmit(ctx, "catalogdb:change", change) })
}
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
func (a *App) LoginWithSession(login, password string, hours int) (application.User, error) {
	return a.service.LoginWithSession(login, password, hours)
}
func (a *App) ResumeSession() (application.Status, error) { return a.service.ResumeSession() }
func (a *App) Logout()                                    { a.service.Logout() }
func (a *App) GetCatalog() (application.Catalog, error)   { return a.service.GetCatalog() }
func (a *App) ExportObjectPDF(kind, id string) (downloads.Entry, error) {
	return a.service.ExportObjectPDF(kind, id)
}
func (a *App) SaveObject(o application.Object) (application.Object, error) {
	return a.service.SaveObject(o)
}
func (a *App) DeleteObject(kind, id string, version int64, fingerprint string) (application.DeletionPreview, error) {
	return a.service.DeleteObjectConfirmed(kind, id, version, fingerprint)
}
func (a *App) PreviewDeletion(kind, id string, version int64) (application.DeletionPreview, error) {
	return a.service.PreviewDeletion(kind, id, version)
}
func (a *App) PreviewClearCatalog() (application.DeletionPreview, error) {
	return a.service.PreviewClearCatalog()
}
func (a *App) ClearCatalog(fingerprint, confirmation string) (application.DeletionPreview, error) {
	return a.service.ClearCatalog(fingerprint, confirmation)
}
func (a *App) SetUserPassword(id, password string) error {
	return a.service.SetUserPassword(id, password)
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

func (a *App) GetAudit(filter application.AuditFilter) (application.AuditPage, error) {
	return a.service.GetAudit(filter)
}

func (a *App) GetStatistics() (application.Statistics, error) { return a.service.GetStatistics() }
func (a *App) GetNotifications(page int, unreadOnly bool) (application.NotificationPage, error) {
	return a.service.GetNotifications(page, unreadOnly)
}
func (a *App) MarkNotificationsRead(ids []string, all bool) error {
	return a.service.MarkNotificationsRead(ids, all)
}

func (a *App) RestoreObject(kind, id string, version, historyID int64, side string) (application.Object, error) {
	return a.service.RestoreObject(kind, id, version, historyID, side)
}

func (a *App) SetImportRowsExcluded(token string, rows []int, exclude bool) (application.ImportPreview, error) {
	return a.service.SetImportRowsExcluded(token, rows, exclude)
}
func (a *App) GetVersions(kind, id string, page int) (application.VersionPage, error) {
	return a.service.GetVersions(kind, id, page)
}
func (a *App) GetRecordVersion(kind, id string, version int64) (any, error) {
	return a.service.GetRecordVersion(kind, id, version)
}
func (a *App) RestoreVersion(kind, id string, currentVersion, targetVersion int64) error {
	return a.service.RestoreVersion(kind, id, currentVersion, targetVersion)
}
func (a *App) GetHistoryPage(kind, id string, page int) (application.HistoryPage, error) {
	return a.service.GetHistoryPage(kind, id, page)
}

func (a *App) ListFilterPresets() ([]application.FilterPreset, error) {
	return a.service.ListFilterPresets()
}
func (a *App) SaveFilterPreset(f application.FilterPreset) (application.FilterPreset, error) {
	return a.service.SaveFilterPreset(f)
}
func (a *App) DeleteFilterPreset(id string, version int64) error {
	return a.service.DeleteFilterPreset(id, version)
}
func (a *App) PreviewBulkEdit(refs []application.ObjectRef, p application.BulkPatch) (application.BulkPreview, error) {
	return a.service.PreviewBulkEdit(refs, p)
}
func (a *App) ApplyBulkEdit(refs []application.ObjectRef, p application.BulkPatch, fingerprint string) (int, error) {
	return a.service.ApplyBulkEdit(refs, p, fingerprint)
}
func (a *App) GetQualityReport(code string, page int) (application.QualityPage, error) {
	return a.service.GetQualityReport(code, page)
}
func (a *App) ListDeleted(page int) (application.DeletedPage, error) {
	return a.service.ListDeleted(page)
}
func (a *App) PreviewDeletedRestore(kind, id string, branch bool) (application.RestorePreview, error) {
	return a.service.PreviewDeletedRestore(kind, id, branch)
}
func (a *App) RestoreDeleted(kind, id string, branch bool, fingerprint string) (int, error) {
	return a.service.RestoreDeleted(kind, id, branch, fingerprint)
}
func (a *App) ExportSelection(ids []string) (downloads.Entry, error) {
	return a.service.ExportSelection(ids)
}

func (a *App) PreviewExcelPolicies(token string, c application.PolicyImportConfig) (application.ImportPreview, error) {
	return a.service.PreviewExcelPolicies(token, c)
}

func (a *App) GetWorkspace() (application.Workspace, error) { return a.service.GetWorkspace() }

func (a *App) BrowseCatalog(r application.CatalogBrowseRequest) (application.CatalogPage, error) {
	return a.service.BrowseCatalog(r)
}
func (a *App) GetCatalogWorkspace() (application.CatalogWorkspace, error) {
	return a.service.GetCatalogWorkspace()
}
func (a *App) GetObjectContext(kind, id string) (application.ObjectContext, error) {
	return a.service.GetObjectContext(kind, id)
}
func (a *App) FindObjects(kind, query string, page int) (application.ObjectOptions, error) {
	return a.service.FindObjects(kind, query, page)
}
func (a *App) SelectCatalogResult(f application.CatalogCriteria) ([]application.ObjectRef, error) {
	return a.service.SelectCatalogResult(f)
}
func (a *App) CountSelectionOutside(f application.CatalogCriteria, ids []string) (int, error) {
	return a.service.CountSelectionOutside(f, ids)
}
func (a *App) ExportCatalogResult(f application.CatalogCriteria) (downloads.Entry, error) {
	return a.service.ExportCatalogResult(f)
}
func (a *App) PreviewMove(r application.MoveRequest) (application.MovePreview, error) {
	return a.service.PreviewMove(r)
}
func (a *App) ApplyMove(r application.MoveRequest, fingerprint string) (application.Object, error) {
	return a.service.ApplyMove(r, fingerprint)
}
func (a *App) GetMovements(kind, id string, page int) (application.MovementPage, error) {
	return a.service.GetMovements(kind, id, page)
}
func (a *App) ListAttachments(kind, id string, page int) (application.AttachmentPage, error) {
	return a.service.ListAttachments(kind, id, page)
}
func (a *App) UploadAttachment(in application.AttachmentUpload) (application.Attachment, error) {
	return a.service.UploadAttachment(in)
}
func (a *App) SaveInstruction(kind, id, attachmentID string, version int64, in documents.Instruction) (application.Attachment, error) {
	return a.service.SaveInstruction(kind, id, attachmentID, version, in)
}
func (a *App) InstructionTemplate(kind, id, template string) (documents.Instruction, error) {
	return a.service.InstructionTemplate(kind, id, template)
}
func (a *App) DeleteAttachment(id string, version int64) error {
	return a.service.DeleteAttachment(id, version)
}
func (a *App) DownloadAttachment(id string) (downloads.Entry, error) {
	return a.service.DownloadAttachment(id)
}

func (a *App) GetPolicyPage(query string, page int) (application.PolicyPage, error) {
	return a.service.GetPolicyPage(query, page)
}
