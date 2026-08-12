package extract

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func jsonFieldLines(data []byte) (map[string]int, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	lines := make(map[string]int)
	seen := make(map[string]struct{})
	if err := parseJSONValue(decoder, data, nil, lines, seen); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err == nil {
		return nil, fmt.Errorf("trailing JSON value")
	} else if err != io.EOF {
		return nil, err
	}
	return lines, nil
}

func parseJSONValue(decoder *json.Decoder, data []byte, path []string, lines map[string]int, seen map[string]struct{}) error {
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
				if _, exists := seen[keyPath]; exists {
					return fmt.Errorf("duplicate JSON key %q", keyPath)
				}
				seen[keyPath] = struct{}{}
				lines[keyPath] = lineAt(data, decoder.InputOffset())
				if err := parseJSONValue(decoder, data, append(path, key), lines, seen); err != nil {
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
				if err := parseJSONValue(decoder, data, itemPath, lines, seen); err != nil {
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

func lineAt(data []byte, offset int64) int {
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	line := 1
	for _, byteValue := range data[:offset] {
		if byteValue == '\n' {
			line++
		}
	}
	return line
}

var _ = json.Delim(0)
