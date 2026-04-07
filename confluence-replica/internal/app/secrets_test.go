package app

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveSecretRefPlainValue(t *testing.T) {
	got, err := resolveSecretRef("plain-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plain-token" {
		t.Fatalf("unexpected token: %q", got)
	}
}

func TestResolveSecretRefKeychainService(t *testing.T) {
	orig := securityExec
	t.Cleanup(func() { securityExec = orig })

	var gotArgs []string
	securityExec = func(args ...string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte("my-pat\n"), nil
	}

	got, err := resolveSecretRef("keychain://codex_confluence_pat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-pat" {
		t.Fatalf("unexpected token: %q", got)
	}

	wantArgs := []string{"find-generic-password", "-s", "codex_confluence_pat", "-w"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected security args: got=%v want=%v", gotArgs, wantArgs)
	}
}

func TestResolveSecretRefKeychainWithAccount(t *testing.T) {
	orig := securityExec
	t.Cleanup(func() { securityExec = orig })

	var gotArgs []string
	securityExec = func(args ...string) ([]byte, error) {
		gotArgs = append([]string{}, args...)
		return []byte("my-pat\n"), nil
	}

	_, err := resolveSecretRef("keychain://codex_confluence_pat?account=oracle-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantArgs := []string{"find-generic-password", "-s", "codex_confluence_pat", "-w", "-a", "oracle-user"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected security args: got=%v want=%v", gotArgs, wantArgs)
	}
}

func TestResolveSecretRefKeychainError(t *testing.T) {
	orig := securityExec
	t.Cleanup(func() { securityExec = orig })

	securityExec = func(args ...string) ([]byte, error) {
		return []byte("not found"), errors.New("exit 44")
	}

	_, err := resolveSecretRef("keychain://missing_service")
	if err == nil {
		t.Fatalf("expected error")
	}
}
