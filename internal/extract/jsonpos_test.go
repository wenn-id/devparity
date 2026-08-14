package extract

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestJSONFieldLines(t *testing.T) {
	data := []byte("{\n" +
		"  \"engines\": {\n" +
		"    \"node\": \">=20 <23\"\n" +
		"  },\n" +
		"  \"packageManager\": \"pnpm@10.0.0\",\n" +
		"  \"scripts\": {\n" +
		"    \"test\": \"node --test\",\n" +
		"    \"build\": \"tsc\"\n" +
		"  }\n" +
		"}\n")

	got, err := jsonFieldLines(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"engines":        2,
		"engines.node":   3,
		"packageManager": 5,
		"scripts":        6,
		"scripts.test":   7,
		"scripts.build":  8,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lines=%#v, want %#v", got, want)
	}
}

func TestJSONFieldLinesRejectsDuplicateKeys(t *testing.T) {
	for _, data := range []string{
		`{"scripts":{"test":"a","test":"b"}}`,
		`{"engines":{"node":"20"},"engines":{"node":"22"}}`,
	} {
		t.Run(data, func(t *testing.T) {
			if _, err := jsonFieldLines([]byte(data)); err == nil {
				t.Fatal("expected duplicate-key error")
			}
		})
	}
}

func TestJSONFieldLinesLargeObjectStaysWithinRuntimeCeiling(t *testing.T) {
	const fields = 50_000
	var builder strings.Builder
	builder.WriteByte('{')
	for index := 0; index < fields; index++ {
		if index > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `"key%d":"value"`, index)
	}
	builder.WriteByte('}')

	started := time.Now()
	lines, err := jsonFieldLines([]byte(builder.String()))
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("large JSON object took %s; want <= 5s", elapsed)
	}
	if len(lines) != fields {
		t.Fatalf("mapped fields=%d, want %d", len(lines), fields)
	}
}

func TestJSONFieldLinesRejectsFieldCountOverCeiling(t *testing.T) {
	var builder strings.Builder
	builder.WriteByte('{')
	for index := 0; index <= maxJSONFields; index++ {
		if index > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `"key%d":0`, index)
	}
	builder.WriteByte('}')

	_, err := jsonFieldLines([]byte(builder.String()))
	if err == nil || !strings.Contains(err.Error(), "field count") {
		t.Fatalf("err=%v, want field-count ceiling", err)
	}
}
