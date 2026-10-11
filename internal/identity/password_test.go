package identity

import "testing"

func TestPasswordHashAndUsernameValidation(t *testing.T) {
	a, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("correct horse battery")
	if err != nil || a == b {
		t.Fatal("missing per-password salt", err)
	}
	for _, test := range []struct {
		hash, password string
		valid          bool
	}{
		{a, "correct horse battery", true}, {a, "wrong", false}, {"", "correct horse battery", false}, {"$argon2id$unbounded", "correct horse battery", false},
	} {
		got, err := VerifyPassword(test.hash, test.password)
		if err != nil || got != test.valid {
			t.Fatal("password validation", got, err)
		}
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
	if n, err := Username(" ALICE "); err != nil || n != "alice" {
		t.Fatal(n, err)
	}
	if _, err := Username("../admin"); err == nil {
		t.Fatal("invalid username accepted")
	}
}
