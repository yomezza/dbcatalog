// Package mobile exposes the existing application service to Android through a
// small gomobile binding. No HTTP server or second implementation of catalog
// rules is involved. Only the explicit UI method list crosses the JSON bridge.
package mobile

import (
	"encoding/json"
	"errors"
	"mime"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/yomezza/test/internal/application"
)

// CredentialStore is implemented by the Android host using Android Keystore.
// A store failure is returned to the user; secrets never fall back to plaintext.
type CredentialStore interface {
	Get(service, key string) (string, error)
	Set(service, key, value string) error
}

// EventSink receives invalidation signals, never catalog records or secrets.
type EventSink interface {
	Emit(schema, eventType string)
}

type nativeCredentials struct{ CredentialStore }

func (s nativeCredentials) Get(service, key string) (string, error) {
	value, err := s.CredentialStore.Get(service, key)
	if err != nil && strings.HasSuffix(err.Error(), "catalogdb.secret.not-found") {
		return "", os.ErrNotExist
	}
	return value, err
}

type Client struct {
	mu      sync.RWMutex
	service *application.Service
	methods map[string]reflect.Value
	closed  bool
}

// NewClient requires an app-private absolute configuration path and a native
// credential store. It deliberately does not call the desktop config.Path/OSStore.
func NewClient(configPath string, store CredentialStore, events EventSink) (*Client, error) {
	if !filepath.IsAbs(configPath) || store == nil {
		return nil, errors.New("Не удалось подготовить защищённое хранилище приложения")
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return nil, errors.New("Не удалось открыть папку приложения")
	}
	s := application.New(configPath, nativeCredentials{store})
	c := &Client{service: s, methods: make(map[string]reflect.Value)}
	source := reflect.ValueOf(s)
	for _, name := range strings.Fields(uiMethods) {
		method := source.MethodByName(name)
		if !method.IsValid() {
			s.Close()
			return nil, errors.New("Несогласованная версия интерфейса приложения")
		}
		c.methods[name] = method
	}
	// These two Wails-facing operations have different service signatures/names.
	c.methods["DeleteObject"] = reflect.ValueOf(s.DeleteObjectConfirmed)
	c.methods["GetPolicyHistory"] = reflect.ValueOf(func(id string) ([]application.HistoryEntry, error) { return s.GetHistory("policy", id) })
	if events != nil {
		s.SetChangeHandler(func(change application.Change) { events.Emit(change.Schema, change.Type) })
	}
	return c, nil
}

// Keep this list explicit. In particular, Close, SetChangeHandler,
// ResolveDownload and GetServerConnection are not exposed to JavaScript.
const uiMethods = `
BrowseCatalog GetCatalogWorkspace GetObjectContext FindObjects SelectCatalogResult
CountSelectionOutside ExportCatalogResult PreviewMove ApplyMove GetMovements
ListAttachments UploadAttachment SaveInstruction InstructionTemplate DeleteAttachment
DownloadAttachment GetPolicyPage GetWorkspace PreviewExcelPolicies ListFilterPresets
SaveFilterPreset DeleteFilterPreset PreviewBulkEdit ApplyBulkEdit GetQualityReport
ListDeleted PreviewDeletedRestore RestoreDeleted ExportSelection SetImportRowsExcluded
GetHistoryPage GetVersions GetRecordVersion RestoreVersion RestoreObject GetStatistics
GetNotifications MarkNotificationsRead ListDownloads ExportToDownloads ExportImportPreview
RevalidateExcel ListPolicies SavePolicy DeletePolicy GetEffectivePolicy PreviewExcel
ApplyExcel ExportExcel GetLookups ListFavorites SetFavorite GetConnection Connect GetStatus
GetDiagnostics Register Login LoginWithSession ResumeSession Logout GetCatalog SaveObject
PreviewDeletion PreviewClearCatalog ClearCatalog GetAudit ListUsers UpdateUser SetUserPassword
GetHistory
`

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// Call accepts a JSON argument array and returns one JSON result envelope.
// Arguments are decoded into the same Go types as desktop calls, including int64
// record versions. It cannot invoke arbitrary methods or access native paths.
func (c *Client) Call(method, arguments string) (response string) {
	defer func() {
		if recover() != nil {
			response = reply(nil, errors.New("Не удалось выполнить операцию приложения"))
		}
	}()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return reply(nil, errors.New("Приложение закрыто"))
	}
	operation, ok := c.methods[method]
	if !ok {
		return reply(nil, errors.New("Неизвестная операция приложения"))
	}
	if len(arguments) > 48*1024*1024 {
		return reply(nil, errors.New("Запрос превышает допустимый размер"))
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil || (strings.TrimSpace(arguments) == "null") {
		return reply(nil, errors.New("Некорректные параметры операции"))
	}
	signature := operation.Type()
	if len(raw) != signature.NumIn() {
		return reply(nil, errors.New("Некорректное число параметров операции"))
	}
	args := make([]reflect.Value, len(raw))
	for i, value := range raw {
		arg := reflect.New(signature.In(i))
		if string(value) == "null" && signature.In(i).Kind() != reflect.Slice && signature.In(i).Kind() != reflect.Pointer && signature.In(i).Kind() != reflect.Map {
			return reply(nil, errors.New("Параметр операции не может быть пустым"))
		}
		if err := json.Unmarshal(value, arg.Interface()); err != nil {
			return reply(nil, errors.New("Некорректный тип параметра операции"))
		}
		args[i] = arg.Elem()
	}
	values := operation.Call(args)
	if len(values) > 0 && signature.Out(len(values)-1).Implements(errorType) {
		last := values[len(values)-1]
		if !last.IsNil() {
			return reply(nil, last.Interface().(error))
		}
		values = values[:len(values)-1]
	}
	if len(values) == 0 {
		return reply(nil, nil)
	}
	return reply(values[0].Interface(), nil)
}

func reply(result any, err error) string {
	if err != nil {
		raw, _ := json.Marshal(struct {
			Error string `json:"error"`
		}{err.Error()})
		return string(raw)
	}
	raw, e := json.Marshal(struct {
		Result any `json:"result"`
	}{result})
	if e != nil {
		return `{"error":"Не удалось подготовить результат операции"}`
	}
	return string(raw)
}

// DownloadInfo is only called by the native host after a user file action.
// JS receives a success result, not an arbitrary filesystem read primitive.
func (c *Client) DownloadInfo(id string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return "", errors.New("Приложение закрыто")
	}
	entry, path, err := c.service.ResolveDownload(id)
	if err != nil {
		return "", err
	}
	contentType := mime.TypeByExtension(filepath.Ext(entry.Name))
	// Android has no desktop /etc/mime.types; Office viewers need an explicit
	// content type to resolve the export intent correctly on a clean device.
	switch strings.ToLower(filepath.Ext(entry.Name)) {
	case ".xlsx":
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".docx":
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	raw, err := json.Marshal(struct {
		Name string `json:"name"`
		Path string `json:"path"`
		MIME string `json:"mime"`
	}{entry.Name, path, contentType})
	return string(raw), err
}

func (c *Client) ConnectionInfo(kind, id string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return "", errors.New("Приложение закрыто")
	}
	target, err := c.service.GetServerConnection(kind, id)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(target)
	return string(raw), err
}

func (c *Client) DiagnosticsReport() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return "", errors.New("Приложение закрыто")
	}
	data, err := json.MarshalIndent(c.service.GetDiagnostics(), "", "  ")
	return string(data) + "\n", err
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		c.service.Close()
	}
}
