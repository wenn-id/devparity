package cli

import "testing"

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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, path, err := normalizePathArgs(tt.args, map[string]bool{"--format": true})
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
