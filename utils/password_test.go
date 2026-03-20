package utils

import (
	"strings"
	"testing"
)

func TestGeneratePassword(t *testing.T) {
	t.Parallel()

	password, err := GeneratePassword(32)
	if err != nil {
		t.Fatalf("GeneratePassword returned error: %v", err)
	}

	if len(password) != 32 {
		t.Fatalf("expected password length 32, got %d", len(password))
	}

	for _, r := range password {
		if !strings.ContainsRune(passwordAlphabet, r) {
			t.Fatalf("password contains disallowed character %q in %q", r, password)
		}
	}
}

func TestGeneratePassword_InvalidLength(t *testing.T) {
	t.Parallel()

	if _, err := GeneratePassword(0); err == nil {
		t.Fatal("expected error for non-positive length")
	}
}

func TestPostgresMD5Hash(t *testing.T) {
	t.Parallel()

	got := PostgresMD5Hash("cloud_admin", "cloud_admin")
	const want = "b093c0d3b281ba6da1eacc608620abd8"
	if got != want {
		t.Fatalf("unexpected hash: got %q, want %q", got, want)
	}
}
