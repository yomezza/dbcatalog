package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/documents"
	"os"
	"strings"
	"testing"
)

func TestAttachmentsAndInstructionsFollowMoveDeletionAndRestore(t *testing.T) {
	s, p, schema := catalogFixture(t)
	_, instance, dbs := workflowBranch(t, s)
	db := dbs[0]
	attachment, err := s.UploadAttachment(AttachmentUpload{Kind: "database", ObjectID: db.ID, Name: "Инструкция PDF", FileName: "guide.pdf", Data: base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 synthetic document"))})
	if err != nil {
		t.Fatal(err)
	}
	template, err := s.InstructionTemplate("database", db.ID, "restore")
	if err != nil || template.Steps == "" {
		t.Fatal(template, err)
	}
	template.Title = "Восстановление <БД>"
	template.Steps += "\nПроверить A & B"
	generated, err := s.SaveInstruction("database", db.ID, "", 0, template)
	if err != nil || generated.Instruction == nil || generated.Version != 1 {
		t.Fatal(generated, err)
	}
	template.Notes = "Особый маркер инструкции"
	updated, err := s.SaveInstruction("database", db.ID, generated.ID, generated.Version, template)
	if err != nil || updated.Version != 2 {
		t.Fatal(updated, err)
	}
	search, err := s.BrowseCatalog(CatalogBrowseRequest{Criteria: CatalogCriteria{Query: "особый маркер инструкции"}})
	if err != nil || search.Counts.Databases != 1 {
		t.Fatal("instruction text is not globally searchable", search, err)
	}
	if _, err = s.SaveInstruction("database", db.ID, generated.ID, generated.Version, template); err == nil {
		t.Fatal("stale instruction overwritten")
	}
	file, err := s.DownloadAttachment(updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, path, err := s.ResolveDownload(file.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	word, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(word.File) != 5 {
		t.Fatal("invalid Word package")
	}
	destination, err := s.SaveObject(Object{Kind: "instance", ParentID: instance.ParentID, Name: "destination", DBMS: "PostgreSQL"})
	if err != nil {
		t.Fatal(err)
	}
	request := MoveRequest{Kind: "database", ID: db.ID, Version: db.Version, ParentID: destination.ID, ParentVersion: destination.Version, Reason: "Перенос учёта"}
	preview, err := s.PreviewMove(request)
	if err != nil || len(preview.Conflicts) != 0 || preview.Attachments != 2 {
		t.Fatal(preview, err)
	}
	moves, err := s.GetMovements("database", db.ID, 0)
	if err != nil || moves.Total != 0 {
		t.Fatal("preview persisted movement", moves, err)
	}
	moved, err := s.ApplyMove(request, preview.Fingerprint)
	if err != nil || moved.ID != db.ID || moved.ParentID != destination.ID {
		t.Fatal(moved, err)
	}
	attached, err := s.ListAttachments("database", db.ID, 0)
	if err != nil || attached.Total != 2 {
		t.Fatal(attached, err)
	}
	moves, err = s.GetMovements("database", db.ID, 0)
	if err != nil || moves.Total != 1 || !strings.Contains(moves.Items[0].FromPath, "pg") || !strings.Contains(moves.Items[0].ToPath, "destination") {
		t.Fatal(moves, err)
	}
	if err = s.RestoreVersion("database", db.ID, moved.Version, 1); err != nil {
		t.Fatal(err)
	}
	moves, err = s.GetMovements("database", db.ID, 0)
	if err != nil || moves.Total != 2 || moves.Items[0].Reason != "Возврат к сохранённой версии" {
		t.Fatal(moves, err)
	}
	if err = s.DeleteObject("database", db.ID, 3); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DownloadAttachment(attachment.ID); err == nil {
		t.Fatal("attachment of a deleted object exposed")
	}
	restoration, err := s.PreviewDeletedRestore("database", db.ID, true)
	if err != nil || len(restoration.Conflicts) > 0 {
		t.Fatal(restoration, err)
	}
	if _, err = s.RestoreDeleted("database", db.ID, true, restoration.Fingerprint); err != nil {
		t.Fatal(err)
	}
	attached, err = s.ListAttachments("database", db.ID, 0)
	if err != nil || attached.Total != 2 {
		t.Fatal(attached, err)
	}
	if _, err = s.DownloadAttachment(attachment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "object_attachments"}.Sanitize()+" SET content='tampered' WHERE id=$1", attachment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DownloadAttachment(attachment.ID); err == nil {
		t.Fatal("attachment integrity failure ignored")
	}
}
func TestAttachmentPermissionsAndInputLimits(t *testing.T) {
	s, p, schema := catalogFixture(t)
	_, _, dbs := workflowBranch(t, s)
	for _, name := range []string{"../guide.pdf", "guide.exe", "a\\b.pdf", "C:guide.pdf"} {
		if _, err := s.UploadAttachment(AttachmentUpload{Kind: "database", ObjectID: dbs[0].ID, Name: "bad", FileName: name, Data: "eA=="}); err == nil {
			t.Fatal("unsafe filename accepted", name)
		}
	}
	if _, err := s.UploadAttachment(AttachmentUpload{Kind: "database", ObjectID: dbs[0].ID, Name: "empty", FileName: "empty.txt", Data: ""}); err == nil {
		t.Fatal("empty file accepted")
	}
	upload := AttachmentUpload{Kind: "database", ObjectID: dbs[0].ID, Name: "guide", FileName: "guide.txt", Data: "eA=="}
	a, err := s.UploadAttachment(upload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET role='viewer' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ListAttachments("database", dbs[0].ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UploadAttachment(upload); err == nil {
		t.Fatal("viewer uploaded attachment")
	}
	if _, err = s.SaveInstruction("database", dbs[0].ID, "", 0, documents.Instruction{Title: "x", Steps: "x"}); err == nil {
		t.Fatal("viewer saved instruction")
	}
	if err = s.DeleteAttachment(a.ID, a.Version); err == nil {
		t.Fatal("viewer removed attachment")
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET role='admin' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteAttachment(a.ID, a.Version+1); err == nil {
		t.Fatal("stale deletion accepted")
	}
	if err = s.DeleteAttachment(a.ID, a.Version); err != nil {
		t.Fatal(err)
	}
}
func TestMoveConflictsPoliciesAndFreshPermissions(t *testing.T) {
	s, p, schema := catalogFixture(t)
	_, source, dbs := workflowBranch(t, s)
	dest, err := s.SaveObject(Object{Kind: "instance", Name: "dest", ParentID: source.ParentID, DBMS: "PostgreSQL"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SavePolicy(Policy{Kind: "instance", ObjectID: source.ID, Name: "Исходная"})
	if err != nil {
		t.Fatal(err)
	}
	afterPolicy, err := s.SavePolicy(Policy{Kind: "instance", ObjectID: dest.ID, Name: "Целевая"})
	if err != nil {
		t.Fatal(err)
	}
	r := MoveRequest{Kind: "database", ID: dbs[0].ID, Version: dbs[0].Version, ParentID: dest.ID, ParentVersion: dest.Version, Reason: "Согласованный перенос"}
	preview, err := s.PreviewMove(r)
	if err != nil || len(preview.Conflicts) > 0 || preview.FromPolicy.Name != "Исходная" || preview.ToPolicy.Name != "Целевая" {
		t.Fatal(preview, err)
	}
	afterPolicy.Description = "Изменена"
	if _, err = s.SavePolicy(afterPolicy); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyMove(r, preview.Fingerprint); err == nil {
		t.Fatal("stale policy preview accepted")
	}
	conflict, err := s.SaveObject(Object{Kind: "database", Name: dbs[0].Name, ParentID: dest.ID})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = s.PreviewMove(r)
	if err != nil || len(preview.Conflicts) == 0 {
		t.Fatal("name conflict missed", preview, err)
	}
	if err = s.DeleteObject("database", conflict.ID, conflict.Version); err != nil {
		t.Fatal(err)
	}
	preview, err = s.PreviewMove(r)
	if err != nil || len(preview.Conflicts) > 0 {
		t.Fatal(preview, err)
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET role='viewer' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyMove(r, preview.Fingerprint); err == nil {
		t.Fatal("role downgrade ignored")
	}
}

func TestMovingInstanceRecordsChildDatabasePathsWithoutChangingTheirIdentity(t *testing.T) {
	s, _, _ := catalogFixture(t)
	_, instance, dbs := workflowBranch(t, s)
	target, err := s.SaveObject(Object{Kind: "server", Name: "new-host"})
	if err != nil {
		t.Fatal(err)
	}
	request := MoveRequest{Kind: "instance", ID: instance.ID, Version: instance.Version, ParentID: target.ID, ParentVersion: target.Version, Reason: "Переезд сервера"}
	preview, err := s.PreviewMove(request)
	if err != nil || preview.Databases != 2 || len(preview.Conflicts) > 0 {
		t.Fatal(preview, err)
	}
	if _, err = s.ApplyMove(request, preview.Fingerprint); err != nil {
		t.Fatal(err)
	}
	child, err := s.GetObjectContext("database", dbs[0].ID)
	if err != nil || child.Object.ParentID != instance.ID || child.Object.Version != dbs[0].Version {
		t.Fatal(child, err)
	}
	history, err := s.GetMovements("database", dbs[0].ID, 0)
	if err != nil || history.Total != 1 || history.Items[0].FromPath != "host / pg / one" || history.Items[0].ToPath != "new-host / pg / one" {
		t.Fatal(history, err)
	}
}
