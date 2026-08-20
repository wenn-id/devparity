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

func TestRedactorDoesNotCorruptLowEntropyForwardedValues(t *testing.T) {
	r := NewRedactor([]string{"1", "1234567890", "18.20.1", "2026-08-20", "1.2.3", "ⅣⅣⅣⅣⅣⅣ", "true", "false", "test", "dev", "UTC", "stable", "normal", "ubuntu", "runner", "python"})
	input := "node 18.20.1 installed in 12 seconds on 2026-08-20; env=test mode=dev zone=UTC channel=stable state=normal roman=ⅣⅣⅣⅣⅣⅣ host=ubuntu runner=runner lang=python"
	if got := r.Redact(input); got != input {
		t.Fatalf("low-entropy values corrupted output: got %q, want %q", got, input)
	}
}

func TestForwardedRedactionEligibilityBoundary(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "abcde", want: false},
		{value: "123456", want: false},
		{value: "18.20.1", want: false},
		{value: "2026-08-20", want: false},
		{value: "ⅣⅣⅣⅣⅣⅣ", want: false},
		{value: "stable", want: false},
		{value: "normal", want: false},
		{value: "ubuntu", want: false},
		{value: "runner", want: false},
		{value: "python", want: false},
		{value: "ubuntu runner", want: false},
		{value: "before", want: false},
		{value: "abc123", want: true},
		{value: "exact-secret", want: true},
		{value: "s3cr3t-value-42", want: true},
	} {
		t.Run(test.value, func(t *testing.T) {
			if got := shouldRedactForwardedValue(test.value); got != test.want {
				t.Fatalf("shouldRedactForwardedValue(%q)=%v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestRedactorStillRemovesSpecificForwardedValues(t *testing.T) {
	const secret = "s3cr3t-value-42"
	r := NewRedactor([]string{secret})
	if got := r.Redact("token=" + secret); got != "token=[REDACTED]" {
		t.Fatalf("specific forwarded value was not redacted: %q", got)
	}
}
