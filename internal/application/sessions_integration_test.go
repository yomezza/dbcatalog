package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/test/internal/config"
)

func resumeClient(t *testing.T, s *Service) *Service {
	t.Helper()
	c, err := config.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	client := New(s.path, s.store)
	t.Cleanup(client.Close)
	if _, err = client.Connect(c, s.pool.Config().ConnConfig.Password, false, false); err != nil {
		t.Fatal(err)
	}
	return client
}
func TestSavedSessionResumesWithoutPasswordAndExpiresOnServer(t *testing.T) {
	s, p, schema := catalogFixture(t)
	s.nextLogin = time.Time{}
	user, err := s.LoginWithSession("admin", "long-enough-password", 8)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.store.Get(sessionStore, s.connectionKey)
	if err != nil || strings.Contains(raw, "long-enough-password") || strings.Contains(raw, "argon2") {
		t.Fatal("unsafe saved credential", err)
	}
	var token savedSession
	if err = json.Unmarshal([]byte(raw), &token); err != nil || len(token.Token) != 43 {
		t.Fatal("missing random token", err)
	}
	var hash string
	var lifetime float64
	if err = p.QueryRow(context.Background(), "SELECT token_hash,extract(epoch FROM(expires_at-created_at)) FROM "+pgx.Identifier{schema, "user_sessions"}.Sanitize()+" WHERE id=$1", token.ID).Scan(&hash, &lifetime); err != nil || hash == token.Token || len(hash) != 64 || lifetime != 8*3600 {
		t.Fatal("server session incorrect", err, lifetime)
	}
	client := resumeClient(t, s)
	status, err := client.ResumeSession()
	if err != nil || status.User == nil || status.User.ID != user.ID {
		t.Fatal("resume", status, err)
	}
	if _, err = client.GetCatalog(); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(context.Background(), "UPDATE "+pgx.Identifier{schema, "user_sessions"}.Sanitize()+" SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 minute' WHERE id=$1", token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetCatalog(); err == nil {
		t.Fatal("expired token authorized a request")
	}
	fresh := resumeClient(t, s)
	status, err = fresh.ResumeSession()
	if err != nil || status.User != nil {
		t.Fatal("expired token resumed", status, err)
	}
	if saved, _ := s.store.Get(sessionStore, s.connectionKey); saved != "" {
		t.Fatal("expired token retained locally")
	}
}
func TestSavedSessionLogoutPasswordAndRoleChanges(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	s.nextLogin = time.Time{}
	if _, err := s.LoginWithSession("admin", "long-enough-password", 4); err != nil {
		t.Fatal(err)
	}
	client := resumeClient(t, s)
	if _, err := client.ResumeSession(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET role='viewer' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	st, err := client.GetStatus()
	if err != nil || st.User == nil || st.User.Role != "viewer" {
		t.Fatal("role not refreshed", st, err)
	}
	if _, err = client.SaveObject(Object{Kind: "server", Name: "forbidden"}); err == nil {
		t.Fatal("saved session retained old admin permissions")
	}
	if _, err = p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET role='admin' WHERE id=$1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetUserPassword(s.user.ID, "changed-admin-password"); err != nil {
		t.Fatal(err)
	}
	st, err = client.GetStatus()
	if err != nil || st.User != nil {
		t.Fatal("password reset did not revoke saved session", st, err)
	}
	s.nextLogin = time.Time{}
	if _, err = s.LoginWithSession("admin", "changed-admin-password", 1); err != nil {
		t.Fatal(err)
	}
	client2 := resumeClient(t, s)
	if _, err = client2.ResumeSession(); err != nil {
		t.Fatal(err)
	}
	s.Logout()
	if _, err = client2.GetCatalog(); err == nil {
		t.Fatal("logout did not revoke token")
	}
	s.nextLogin = time.Time{}
	if _, err = s.LoginWithSession("admin", "changed-admin-password", 1); err != nil {
		t.Fatal(err)
	}
	id := s.user.ID
	if _, err = p.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "users"}.Sanitize()+" SET state='blocked' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	st, err = s.ResumeSession()
	if err != nil || st.User != nil {
		t.Fatal("blocked account resumed", st, err)
	}
}

type failingSessionStore struct{ CredentialStore }

func (f failingSessionStore) Set(service, key, value string) error {
	if service == sessionStore && value != "" {
		return errors.New("synthetic secure store failure")
	}
	return f.CredentialStore.Set(service, key, value)
}
func TestSavedSessionOptOutIsolationAndSecureStoreFailure(t *testing.T) {
	s, p, schema := catalogFixture(t)
	ctx := context.Background()
	for _, hours := range []int{-1, 169} {
		if _, err := s.LoginWithSession("admin", "long-enough-password", hours); err == nil {
			t.Fatal("invalid lifetime accepted")
		}
	}
	s.nextLogin = time.Time{}
	if _, err := s.LoginWithSession("admin", "long-enough-password", 0); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, "user_sessions"}.Sanitize()).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if st, err := resumeClient(t, s).ResumeSession(); err != nil || st.User != nil {
		t.Fatal("opt-out persisted login", st, err)
	}
	s.nextLogin = time.Time{}
	if _, err := s.LoginWithSession("admin", "long-enough-password", 1); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	c.Schema = schema + "_other"
	other := New(s.path, s.store)
	t.Cleanup(other.Close)
	t.Cleanup(func() { _, _ = p.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{c.Schema}.Sanitize()+" CASCADE") })
	if _, err = other.Connect(c, s.pool.Config().ConnConfig.Password, false, false); err != nil {
		t.Fatal(err)
	}
	if st, err := other.ResumeSession(); err != nil || st.User != nil {
		t.Fatal("token crossed schemas", st, err)
	}
	s.Logout()
	s.store = failingSessionStore{s.store}
	s.nextLogin = time.Time{}
	if _, err = s.LoginWithSession("admin", "long-enough-password", 2); err == nil {
		t.Fatal("secure store failure ignored")
	}
	if s.user != nil {
		t.Fatal("failed persistent login left authenticated user")
	}
	if err = p.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{schema, "user_sessions"}.Sanitize()+" WHERE revoked_at IS NULL").Scan(&n); err != nil || n != 0 {
		t.Fatal("unpersisted token survived rollback", n, err)
	}
}

func TestCorruptSavedSessionFallsBackToLogin(t *testing.T) {
	s, _, _ := catalogFixture(t)
	for _, raw := range []string{"{broken", `{"id":"not-a-uuid","token":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`} {
		if err := s.store.Set(sessionStore, s.connectionKey, raw); err != nil {
			t.Fatal(err)
		}
		client := resumeClient(t, s)
		st, err := client.ResumeSession()
		if err != nil || st.User != nil {
			t.Fatal("corrupt saved entry prevented normal login", st, err)
		}
	}
}
