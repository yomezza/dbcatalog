package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/documents"
	"github.com/yomezza/test/internal/downloads"
	"github.com/yomezza/test/internal/security"
	"path/filepath"
	"strings"
	"time"
)

const MaxAttachmentSize = 20 * 1024 * 1024

type Attachment struct {
	ID          string                 `json:"id"`
	Kind        string                 `json:"kind"`
	ObjectID    string                 `json:"objectId"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	FileName    string                 `json:"fileName"`
	MIME        string                 `json:"mime"`
	Size        int64                  `json:"size"`
	SHA256      string                 `json:"sha256"`
	Version     int64                  `json:"version"`
	Actor       string                 `json:"actor"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	Instruction *documents.Instruction `json:"instruction"`
}
type AttachmentPage struct {
	Items []Attachment `json:"items"`
	Total int64        `json:"total"`
	Page  int          `json:"page"`
}
type AttachmentUpload struct {
	Kind        string `json:"kind"`
	ObjectID    string `json:"objectId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	FileName    string `json:"fileName"`
	Data        string `json:"data"`
}

var attachmentTypes = map[string]string{".pdf": "application/pdf", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".sql": "text/plain", ".ps1": "text/plain", ".json": "application/json", ".zip": "application/zip"}

func attachmentFilename(name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 180 || strings.ContainsAny(name, "/\\\x00:") || name == "" || name == "." || name == ".." {
		return "", "", errors.New("Некорректное имя вложения")
	}
	ext := strings.ToLower(filepath.Ext(name))
	mime, ok := attachmentTypes[ext]
	if !ok {
		return "", "", errors.New("Поддерживаются PDF, DOCX, XLSX, TXT, Markdown, CSV, изображения PNG/JPEG, SQL, PS1, JSON и ZIP")
	}
	return name, mime, nil
}
func scanAttachment(row pgx.Row) (Attachment, error) {
	var a Attachment
	var document []byte
	err := row.Scan(&a.ID, &a.Kind, &a.ObjectID, &a.Name, &a.Description, &a.FileName, &a.MIME, &a.Size, &a.SHA256, &a.Version, &a.Actor, &a.UpdatedAt, &document)
	if err == nil && len(document) > 0 && string(document) != "null" {
		var in documents.Instruction
		if err = json.Unmarshal(document, &in); err == nil {
			a.Instruction = &in
		}
	}
	return a, err
}

const attachmentColumns = `a.id::text,a.entity,a.object_id::text,a.name,a.description,a.file_name,a.mime_type,octet_length(a.content)::bigint,a.sha256,a.version,u.login,a.updated_at,a.document`

func (s *Service) ListAttachments(kind, id string, page int) (AttachmentPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := AttachmentPage{Items: []Attachment{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = loadObject(ctx, tx, kind, id, false); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM object_attachments WHERE entity=$1 AND object_id=$2`, kind, id).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, "SELECT "+attachmentColumns+` FROM object_attachments a JOIN users u ON u.id=a.updated_by WHERE a.entity=$1 AND a.object_id=$2 ORDER BY a.created_at DESC,a.id LIMIT 25 OFFSET $3`, kind, id, out.Page*25)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		a, e := scanAttachment(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, a)
	}
	return out, rows.Err()
}
func (s *Service) saveAttachment(ctx context.Context, tx pgx.Tx, kind, id, name, description, file, mime string, data []byte, in *documents.Instruction, existing string, version int64) (Attachment, error) {
	if _, err := loadObject(ctx, tx, kind, id, true); err != nil {
		return Attachment{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 200 || len(description) > 20000 {
		return Attachment{}, errors.New("Укажите название до 200 символов и описание до 20000 символов")
	}
	if len(data) == 0 || len(data) > MaxAttachmentSize {
		return Attachment{}, errors.New("Размер вложения: от 1 байта до 20 МБ")
	}
	sha := fmt.Sprintf("%x", sha256.Sum256(data))
	var document any
	if in != nil {
		raw, _ := json.Marshal(in)
		document = raw
	}
	action := "attachment_add"
	if existing == "" {
		existing = security.ID()
		_, err := tx.Exec(ctx, `INSERT INTO object_attachments(id,entity,object_id,name,description,file_name,mime_type,content,sha256,document,created_by,updated_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)`, existing, kind, id, name, description, file, mime, data, sha, document, s.user.ID)
		if err != nil {
			return Attachment{}, importDatabaseError(err)
		}
	} else {
		action = "instruction_update"
		tag, err := tx.Exec(ctx, `UPDATE object_attachments SET name=$4,description=$5,file_name=$6,mime_type=$7,content=$8,sha256=$9,document=$10,updated_by=$11,updated_at=now(),version=version+1 WHERE id=$1 AND entity=$2 AND object_id=$3 AND version=$12 AND document IS NOT NULL`, existing, kind, id, name, description, file, mime, data, sha, document, s.user.ID, version)
		if err != nil {
			return Attachment{}, importDatabaseError(err)
		}
		if tag.RowsAffected() != 1 {
			return Attachment{}, errors.New("Инструкция изменена или удалена. Обновите список")
		}
	}
	details, _ := json.Marshal(map[string]any{"attachmentId": existing, "name": name, "fileName": file, "size": len(data)})
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id,details)VALUES($1,$2,$3,$4,$5)`, s.user.ID, action, kind, id, details); err != nil {
		return Attachment{}, err
	}
	return scanAttachment(tx.QueryRow(ctx, "SELECT "+attachmentColumns+` FROM object_attachments a JOIN users u ON u.id=a.updated_by WHERE a.id=$1`, existing))
}
func (s *Service) UploadAttachment(in AttachmentUpload) (Attachment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(in.Data) > ((MaxAttachmentSize+2)/3)*4 {
		return Attachment{}, errors.New("Максимальный размер вложения — 20 МБ")
	}
	name, mime, err := attachmentFilename(in.FileName)
	if err != nil {
		return Attachment{}, err
	}
	data, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		return Attachment{}, errors.New("Некорректное содержимое файла")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return Attachment{}, err
	}
	defer tx.Rollback(ctx)
	a, err := s.saveAttachment(ctx, tx, in.Kind, in.ObjectID, in.Name, in.Description, name, mime, data, nil, "", 0)
	if err != nil {
		return a, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Attachment{}, importDatabaseError(err)
	}
	return a, nil
}
func (s *Service) SaveInstruction(kind, id, attachmentID string, version int64, in documents.Instruction) (Attachment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return Attachment{}, err
	}
	defer tx.Rollback(ctx)
	object, err := loadObjectContext(ctx, tx, kind, id)
	if err != nil {
		return Attachment{}, err
	}
	fields := []documents.Field{{Label: "Объект", Value: object.Path}, {Label: "Версия записи", Value: fmt.Sprint(object.Object.Version)}, {Label: "Автор", Value: s.user.Login}, {Label: "Дата формирования", Value: time.Now().Format(time.RFC3339)}}
	for _, server := range object.Catalog.Servers {
		fields = append(fields, documents.Field{Label: "Адрес сервера", Value: server.Address}, documents.Field{Label: "Подключение", Value: server.Attributes.ConnectionMethod})
	}
	fields = append(fields, documents.Field{Label: "Ответственный", Value: object.Object.Attributes.Responsible})
	data, err := documents.Word(in, fields)
	if err != nil {
		return Attachment{}, err
	}
	title := []rune(in.Title)
	if len(title) > 110 {
		title = title[:110]
	}
	file := "Инструкция-" + strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, string(title)) + ".docx"
	a, err := s.saveAttachment(ctx, tx, kind, id, in.Title, "Инструкция: "+map[string]string{"connection": "Подключение", "backup": "Резервное копирование", "restore": "Восстановление", "maintenance": "Регламент"}[in.Template], file, attachmentTypes[".docx"], data, &in, attachmentID, version)
	if err != nil {
		return a, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Attachment{}, importDatabaseError(err)
	}
	return a, nil
}
func (s *Service) DeleteAttachment(id string, version int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var kind, objectID, name string
	if err = tx.QueryRow(ctx, `SELECT entity,object_id::text,name FROM object_attachments WHERE id=$1 AND version=$2 FOR UPDATE`, id, version).Scan(&kind, &objectID, &name); err != nil {
		return errors.New("Вложение изменено или удалено")
	}
	if _, err = loadObject(ctx, tx, kind, objectID, true); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM object_attachments WHERE id=$1`, id); err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]string{"attachmentId": id, "name": name})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,entity,object_id,details)VALUES($1,'attachment_delete',$2,$3,$4)`, s.user.ID, kind, objectID, details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) DownloadAttachment(id string) (downloads.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := s.downloadStore(ctx)
	if err != nil {
		return downloads.Entry{}, err
	}
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return downloads.Entry{}, err
	}
	defer tx.Rollback(ctx)
	var kind, objectID, file, sha string
	var data []byte
	if err = tx.QueryRow(ctx, `SELECT entity,object_id::text,file_name,sha256,content FROM object_attachments WHERE id=$1`, id).Scan(&kind, &objectID, &file, &sha, &data); err != nil {
		return downloads.Entry{}, errors.New("Вложение не найдено")
	}
	if _, err = loadObject(ctx, tx, kind, objectID, false); err != nil {
		return downloads.Entry{}, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != sha {
		return downloads.Entry{}, errors.New("Нарушена целостность вложения")
	}
	return store.Add(file, "Вложение объекта", data)
}
