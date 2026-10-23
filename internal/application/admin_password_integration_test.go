package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/security"
)

func TestAdministratorChangesOtherAndOwnPassword(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	actor := s.user.ID
	if err := s.Register("targetuser", "original-password", false); err != nil {
		t.Fatal(err)
	}
	var target string
	if err := p.QueryRow(ctx, "SELECT id::text FROM "+pgx.Identifier{schema, "users"}.Sanitize()+" WHERE login='targetuser'").Scan(&target); err != nil {
		t.Fatal(err)
	}
	// Pending accounts can receive a new password without being activated.
	if err := s.SetUserPassword(target, "new-target-password"); err != nil {
		t.Fatal(err)
	}
	var hash, role, state string
	if err := p.QueryRow(ctx, "SELECT password_hash,role,state FROM "+pgx.Identifier{schema, "users"}.Sanitize()+" WHERE id=$1", target).Scan(&hash, &role, &state); err != nil {
		t.Fatal(err)
	}
	if !security.Verify(hash, "new-target-password") || security.Verify(hash, "original-password") || role != "viewer" || state != "pending" {
		t.Fatal("password or account state incorrect")
	}
	var raw []byte
	if err := p.QueryRow(ctx, "SELECT to_jsonb(a) FROM "+pgx.Identifier{schema, "audit_events"}.Sanitize()+" a WHERE action='user_password_change' AND object_id=$1", target).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "new-target-password") || strings.Contains(string(raw), hash) || strings.Contains(string(raw), "argon2") {
		t.Fatal("password/hash in audit")
	}
	var audit struct {
		ActorID string         `json:"actor_id"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(raw, &audit); err != nil || audit.ActorID != actor || len(audit.Details) != 0 {
		t.Fatal(string(raw), err)
	}
	var body string
	if err := p.QueryRow(ctx, "SELECT body FROM "+pgx.Identifier{schema, "notifications"}.Sanitize()+" WHERE user_id=$1 AND title='Пароль изменён'", target).Scan(&body); err != nil || strings.Contains(body, "new-target-password") {
		t.Fatal(body, err)
	}
	if err := s.UpdateUser(target, "viewer", "active"); err != nil {
		t.Fatal(err)
	}
	// Exercise actual authentication, not only hash verification.
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err := s.Login("targetuser", "original-password"); err == nil {
		t.Fatal("old password accepted")
	}
	s.nextLogin = time.Time{}
	if _, err := s.Login("targetuser", "new-target-password"); err != nil {
		t.Fatal(err)
	}
	s.nextLogin = time.Time{}
	if _, err := s.Login("admin", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserPassword(actor, "new-admin-password"); err != nil {
		t.Fatal(err)
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err := s.Login("admin", "new-admin-password"); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordChangeRequiresFreshAdministratorAndRollsBackAuditFailure(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	actor := s.user.ID
	users := pgx.Identifier{schema, "users"}.Sanitize()
	readHash := func() string {
		t.Helper()
		var hash string
		if err := p.QueryRow(ctx, "SELECT password_hash FROM "+users+" WHERE id=$1", actor).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		return hash
	}
	before := readHash()
	for _, password := range []string{"", "short", strings.Repeat("x", 1025)} {
		if err := s.SetUserPassword(actor, password); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if err := s.SetUserPassword(security.ID(), "valid-new-password"); err == nil {
		t.Fatal("missing user accepted")
	}
	for _, role := range []string{"editor", "viewer"} {
		if _, err := p.Exec(ctx, "UPDATE "+users+" SET role=$2 WHERE id=$1", actor, role); err != nil {
			t.Fatal(err)
		}
		if err := s.SetUserPassword(actor, "valid-new-password"); err == nil {
			t.Fatal("non-admin reset accepted", role)
		}
	}
	if _, err := p.Exec(ctx, "UPDATE "+users+" SET role='admin',state='blocked' WHERE id=$1", actor); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserPassword(actor, "valid-new-password"); err == nil {
		t.Fatal("blocked administrator accepted")
	}
	if _, err := p.Exec(ctx, "UPDATE "+users+" SET state='active' WHERE id=$1", actor); err != nil {
		t.Fatal(err)
	}
	// Ensure changing the hash and recording audit/notification remain atomic.
	_, err := p.Exec(ctx, "CREATE FUNCTION "+pgx.Identifier{schema, "reject_password_audit"}.Sanitize()+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='user_password_change' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF;RETURN NEW;END $$; CREATE TRIGGER reject_password_audit BEFORE INSERT ON `+pgx.Identifier{schema, "audit_events"}.Sanitize()+" FOR EACH ROW EXECUTE FUNCTION "+pgx.Identifier{schema, "reject_password_audit"}.Sanitize()+"()")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetUserPassword(actor, "valid-new-password"); err == nil {
		t.Fatal("audit failure accepted")
	}
	if readHash() != before {
		t.Fatal("failed reset changed password")
	}
	var n int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, "notifications"}.Sanitize()+" WHERE title='Пароль изменён'").Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	s.Logout()
	if err = s.SetUserPassword(actor, "valid-new-password"); err == nil {
		t.Fatal("anonymous reset accepted")
	}
}
