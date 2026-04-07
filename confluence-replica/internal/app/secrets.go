package app

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

func resolveSecretRef(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if !strings.HasPrefix(trimmed, "keychain://") {
		return trimmed, nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse keychain url: %w", err)
	}

	service := strings.TrimSpace(u.Host)
	if service == "" {
		service = strings.Trim(strings.TrimSpace(u.Path), "/")
	}
	if service == "" {
		return "", fmt.Errorf("keychain service is empty in %q", trimmed)
	}

	args := []string{"find-generic-password", "-s", service, "-w"}
	if account := strings.TrimSpace(u.Query().Get("account")); account != "" {
		args = append(args, "-a", account)
	}
	out, err := securityExec(args...)
	if err != nil {
		return "", fmt.Errorf("security command failed for service %q: %w", service, err)
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("empty secret returned from keychain service %q", service)
	}
	return token, nil
}

var securityExec = func(args ...string) ([]byte, error) {
	return exec.Command("security", args...).CombinedOutput()
}
