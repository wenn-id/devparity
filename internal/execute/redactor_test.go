package execute

import (
	"strings"
	"testing"
)

func TestNewHostGrantRequiresTrust(t *testing.T) {
	if _, err := NewHostGrant(false); err == nil {
		t.Fatal("expected trust error")
	}
}

func TestRedactorRemovesForwardedAndKnownTokens(t *testing.T) {
	r := NewRedactor([]string{"exact-secret"})
	got := r.Redact("exact-secret ghp_abcdefghijklmnopqrstuvwxyz123456 npm_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx Bearer abc.def-123")
	for _, secret := range []string{"exact-secret", "ghp_", "npm_", "Bearer abc.def-123"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q remained in %q", secret, got)
		}
	}
	if strings.Contains(r.Redact("-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----"), "PRIVATE KEY") {
		t.Fatal("PEM body remained")
	}
}
