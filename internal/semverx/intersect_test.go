package semverx

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare major", "  v22  ", "22.x"},
		{"operator bare majors", ">=v20	<v23", ">=20.x <23.x"},
		{"range and wildcard", "V18.X || v22.4.x", "18.X || 22.4.x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in)
			if err != nil || got != tt.want {
				t.Fatalf("Normalize(%q)=%q, err=%v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestNormalizeRejectsEmptyAndPrerelease(t *testing.T) {
	for _, raw := range []string{"", "   ", "22.0.0-beta.1", ">=20 <22.0.0-rc.1"} {
		if got, err := Normalize(raw); err == nil {
			t.Fatalf("Normalize(%q)=%q, want error", raw, got)
		}
	}
}

func TestIntersectsAll(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want bool
	}{
		{"overlap", []string{">=20 <23", "22.x"}, true},
		{"disjoint", []string{">=20 <22", "22.x"}, false},
		{"exact", []string{"22.4.1", "^22.0.0"}, true},
		{"gap", []string{">1.2.3 <1.2.4", "1.x"}, false},
		{"or", []string{"18.x || 22.x", ">=21 <23"}, true},
		{"caret zero major", []string{"^0.2.3", "0.2.x"}, true},
		{"caret zero patch disjoint", []string{"^0.0.3", "0.0.0 - 0.0.2"}, false},
		{"tilde", []string{"~18.4.0", ">=18.4.1 <18.5.0"}, true},
		{"hyphen", []string{"20.1.0 - 22.9.9", "^22.0.0"}, true},
		{"wildcard upper boundary", []string{"20.x", ">=23"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IntersectsAll(tt.in)
			if err != nil || got != tt.want {
				t.Fatalf("%s: got=%v err=%v", tt.name, got, err)
			}
		})
	}
}

func TestIntersectsAllRejectsEmptyAndPrerelease(t *testing.T) {
	for _, raw := range [][]string{{}, {""}, {"22.0.0-beta.1"}} {
		if got, err := IntersectsAll(raw); err == nil {
			t.Fatalf("IntersectsAll(%q)=%v, want error", raw, got)
		}
	}
}
