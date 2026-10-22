package security

import "testing"

func TestPasswordHash(t *testing.T) {
	hash, err := Hash("long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(hash, "long-enough-password") || Verify(hash, "wrong-password") {
		t.Fatal("password verification failed")
	}
	for _, bad := range []string{"", "$argon2id$v=19$m=999999999,t=3,p=2$abc$abc", hash + "$extra"} {
		if Verify(bad, "long-enough-password") {
			t.Fatal("invalid hash accepted")
		}
	}
	if _, err = Hash("short"); err == nil {
		t.Fatal("short password accepted")
	}
}
