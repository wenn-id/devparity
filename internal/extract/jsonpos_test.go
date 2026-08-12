package extract

import (
	"reflect"
	"testing"
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
