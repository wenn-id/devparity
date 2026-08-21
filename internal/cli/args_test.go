package cli

import (
	"strings"
	"testing"
)

func TestNormalizePathArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantFlags []string
		wantPath  string
		wantErr   bool
	}{
		{name: "bool before path", args: []string{"--strict", "repo"}, wantFlags: []string{"--strict"}, wantPath: "repo"},
		{name: "bool after path", args: []string{"repo", "--strict"}, wantFlags: []string{"--strict"}, wantPath: "repo"},
		{name: "value before path", args: []string{"--format", "json", "repo"}, wantFlags: []string{"--format", "json"}, wantPath: "repo"},
		{name: "value after path", args: []string{"repo", "--format", "json"}, wantFlags: []string{"--format", "json"}, wantPath: "repo"},
		{name: "two paths", args: []string{"one", "two"}, wantErr: true},
		{name: "missing value", args: []string{"--format"}, wantErr: true},
		{name: "unknown flag", args: []string{"--nope", "repo"}, wantErr: true},
		{name: "empty value via equals", args: []string{"--format=", "repo"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, path, err := normalizePathArgs(tt.args, subcommandSpec{name: "doctor", boolFlags: map[string]bool{"--strict": true}, valueFlags: map[string]bool{"--format": true}})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if path != tt.wantPath {
				t.Fatalf("path=%q, want %q", path, tt.wantPath)
			}
			if len(flags) != len(tt.wantFlags) {
				t.Fatalf("flags=%#v, want %#v", flags, tt.wantFlags)
			}
			for i := range flags {
				if flags[i] != tt.wantFlags[i] {
					t.Fatalf("flags=%#v, want %#v", flags, tt.wantFlags)
				}
			}
		})
	}
}

func TestNormalizePathArgsScopesFlagsPerSubcommand(t *testing.T) {
	// A flag owned by another subcommand must be rejected here, with a
	// message that names the subcommand where it does work.
	_, _, err := normalizePathArgs([]string{"--execute", "repo"}, doctorFlags)
	if err == nil || !strings.Contains(err.Error(), "docs verify") {
		t.Fatalf("err=%v, want --execute rejected with a pointer to docs verify", err)
	}
	_, _, err = normalizePathArgs([]string{"--strict", "repo"}, docsVerifyFlags)
	if err == nil || !strings.Contains(err.Error(), "doctor") {
		t.Fatalf("err=%v, want --strict rejected with a pointer to doctor", err)
	}
	// Flags this subcommand owns must keep working.
	flags, path, err := normalizePathArgs([]string{"--strict", "repo"}, doctorFlags)
	if err != nil || path != "repo" || len(flags) != 1 || flags[0] != "--strict" {
		t.Fatalf("flags=%#v path=%q err=%v, want --strict accepted on doctor", flags, path, err)
	}
	_, path, err = normalizePathArgs([]string{"--container", "repo"}, docsVerifyFlags)
	if err != nil || path != "repo" {
		t.Fatalf("path=%q err=%v, want --container accepted on docs verify", path, err)
	}
}

func TestSubcommandUsageUsesDoubleDashStyle(t *testing.T) {
	usage := doctorFlags.Usage()
	for _, required := range []string{"devparity doctor", "--strict", "--format <value>"} {
		if !strings.Contains(usage, required) {
			t.Fatalf("usage=%q, missing %q", usage, required)
		}
	}
	if strings.Contains(usage, " -strict") || strings.Contains(usage, " -format") {
		t.Fatalf("usage=%q advertises single-dash flags the CLI does not accept", usage)
	}
}
