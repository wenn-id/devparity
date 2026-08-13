package cli

import (
	"fmt"
	"strings"
)

var knownBoolFlags = map[string]bool{
	"--strict":           true,
	"--execute":          true,
	"--trust-repository": true,
	"--container":        true,
	"--allow-network":    true,
}

func normalizePathArgs(args []string, valueFlags map[string]bool) ([]string, string, error) {
	var flags []string
	var path string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name, value, hasValue := strings.Cut(arg, "=")
			if hasValue {
				if !valueFlags[name] || value == "" {
					return nil, "", fmt.Errorf("invalid flag %q", arg)
				}
				flags = append(flags, name, value)
				continue
			}
			if valueFlags[arg] {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
					return nil, "", fmt.Errorf("flag %s requires a value", arg)
				}
				flags = append(flags, arg, args[i+1])
				i++
				continue
			}
			if !knownBoolFlags[arg] {
				return nil, "", fmt.Errorf("unknown flag %s", arg)
			}
			flags = append(flags, arg)
			continue
		}
		if path != "" {
			return nil, "", fmt.Errorf("multiple paths supplied")
		}
		path = arg
	}
	return flags, path, nil
}
