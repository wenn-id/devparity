package extract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	maxJSONFields         = 100_000
	maxJSONFieldPathBytes = 4 << 10
	maxJSONDepth          = 256
)

func jsonFieldLines(data []byte) (map[string]int, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	lineStarts := buildLineStarts(data)
	lines := make(map[string]int)
	if err := parseJSONValue(decoder, lineStarts, nil, lines, 0); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err == nil {
		return nil, fmt.Errorf("trailing JSON value")
	} else if err != io.EOF {
		return nil, err
	}
	return lines, nil
}

func parseJSONValue(decoder *json.Decoder, lineStarts []int, path []string, lines map[string]int, depth int) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("JSON nesting depth exceeds maximum %d", maxJSONDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				keyPath := appendPath(path, key)
				if len(keyPath) > maxJSONFieldPathBytes {
					return fmt.Errorf("JSON field path exceeds maximum length %d bytes", maxJSONFieldPathBytes)
				}
				if _, exists := lines[keyPath]; exists {
					return fmt.Errorf("duplicate JSON key %q", keyPath)
				}
				if len(lines) >= maxJSONFields {
					return fmt.Errorf("JSON field count exceeds maximum %d", maxJSONFields)
				}
				lines[keyPath] = lineAt(lineStarts, decoder.InputOffset())
				if err := parseJSONValue(decoder, lineStarts, append(path, key), lines, depth+1); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil {
				return err
			}
			if end != json.Delim('}') {
				return fmt.Errorf("expected object end")
			}
		case '[':
			index := 0
			for decoder.More() {
				itemPath := append(path, fmt.Sprintf("[%d]", index))
				if err := parseJSONValue(decoder, lineStarts, itemPath, lines, depth+1); err != nil {
					return err
				}
				index++
			}
			end, err := decoder.Token()
			if err != nil {
				return err
			}
			if end != json.Delim(']') {
				return fmt.Errorf("expected array end")
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", value)
		}
	}
	return nil
}

func appendPath(path []string, part string) string {
	if len(path) == 0 {
		return part
	}
	return strings.Join(path, ".") + "." + part
}

func buildLineStarts(data []byte) []int {
	starts := []int{0}
	for index, byteValue := range data {
		if byteValue == '\n' {
			starts = append(starts, index+1)
		}
	}
	return starts
}

func lineAt(lineStarts []int, offset int64) int {
	if offset < 0 {
		offset = 0
	}
	if len(lineStarts) == 0 {
		return 1
	}
	if offset > int64(lineStarts[len(lineStarts)-1]) {
		offset = int64(lineStarts[len(lineStarts)-1])
	}
	line := sort.Search(len(lineStarts), func(index int) bool {
		return int64(lineStarts[index]) > offset
	})
	if line == 0 {
		return 1
	}
	return line
}

var _ = json.Delim(0)
