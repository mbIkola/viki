package app

import (
	"strings"
	"testing"
)

func TestSanitizeDSNMasksPassword(t *testing.T) {
	in := "postgres://alice:secret@localhost:5432/confluence_replica?sslmode=disable"
	got := sanitizeDSN(in)
	if got == in {
		t.Fatalf("expected sanitized dsn to differ from input")
	}
	if got == "" {
		t.Fatalf("expected non-empty dsn")
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("password leaked in dsn: %s", got)
	}
}
