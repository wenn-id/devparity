package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONRendersSchemaAndTrailingNewline(t *testing.T) {
	var out bytes.Buffer
	if err := JSON(&out, fixedReport()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("JSON=%q, want trailing newline", out.String())
	}
	var decoded struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != 1 {
		t.Fatalf("schema_version=%d, want 1", decoded.SchemaVersion)
	}
}
