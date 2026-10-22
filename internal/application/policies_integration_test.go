package application

import (
	"testing"
	"time"
)

func TestBackupPoliciesInheritanceValidationAndPermissions(t *testing.T) {
	s, _, _ := catalogFixture(t)
	server, err := s.SaveObject(Object{Kind: "server", Name: "host"})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := s.SaveObject(Object{Kind: "instance", Name: "pg", ParentID: server.ID, DBMS: "PostgreSQL"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.SaveObject(Object{Kind: "database", Name: "db", ParentID: instance.ID})
	if err != nil {
		t.Fatal(err)
	}
	effective, err := s.GetEffectivePolicy(db.ID)
	if err != nil || effective.Policy != nil {
		t.Fatal("unexpected effective policy", err)
	}
	p, err := s.SavePolicy(Policy{Kind: "instance", ObjectID: instance.ID, Name: "Общая", Enabled: true, BackupType: "Локальное и Централизованное", Scheduler: "cron", Schedule: "Ежедневно в 02:00", Storage: "/backup", RetentionDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	effective, err = s.GetEffectivePolicy(db.ID)
	if err != nil || effective.Policy == nil || effective.Policy.ID != p.ID || !effective.Inherited {
		t.Fatal("instance policy not inherited", err)
	}
	direct, err := s.SavePolicy(Policy{Kind: "database", ObjectID: db.ID, Name: "Отключить РК"})
	if err != nil {
		t.Fatal(err)
	}
	effective, err = s.GetEffectivePolicy(db.ID)
	if err != nil || effective.Policy == nil || effective.Policy.ID != direct.ID || effective.Policy.Enabled || effective.Inherited {
		t.Fatal("disabled direct policy must override inherited policy", err)
	}
	incomplete := direct
	incomplete.Enabled = true
	if _, err = s.SavePolicy(incomplete); err == nil {
		t.Fatal("incomplete enabled policy accepted")
	}
	if _, err = s.SavePolicy(Policy{Kind: "database", ObjectID: db.ID, Name: "Дубликат"}); err == nil {
		t.Fatal("two direct policies accepted for one database")
	}
	stale := direct
	direct.Description = "Изменено"
	direct, err = s.SavePolicy(direct)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePolicy(stale); err == nil {
		t.Fatal("stale policy update accepted")
	}
	events, err := s.GetHistory("policy", direct.ID)
	if err != nil || len(events) != 2 {
		t.Fatal("policy history missing", err)
	}
	if err = s.DeletePolicy(direct.ID, direct.Version); err != nil {
		t.Fatal(err)
	}
	effective, err = s.GetEffectivePolicy(db.ID)
	if err != nil || effective.Policy == nil || effective.Policy.ID != p.ID || !effective.Inherited {
		t.Fatal("inheritance not restored after direct deletion", err)
	}
	if err = s.Register("viewer", "long-enough-password", false); err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Login == "viewer" {
			if err = s.UpdateUser(u.ID, "viewer", "active"); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.Logout()
	s.nextLogin = time.Time{}
	if _, err = s.Login("viewer", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ListPolicies(); err != nil {
		t.Fatal("viewer cannot view", err)
	}
	if _, err = s.SavePolicy(p); err == nil {
		t.Fatal("viewer edited policy")
	}
	if err = s.DeletePolicy(p.ID, p.Version); err == nil {
		t.Fatal("viewer deleted policy")
	}
}
