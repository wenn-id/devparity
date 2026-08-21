package cli

import (
	"fmt"
	"sort"
	"strings"
)

// subcommandSpec describes the flags one subcommand accepts. The bool and
// value flag sets are scoped per subcommand so a flag that belongs to another
// subcommand is rejected here, with a helpful message, instead of leaking Go
// flag-package usage text that advertises single-dash names the CLI never
// accepts.
type subcommandSpec struct {
	name       string
	boolFlags  map[string]bool
	valueFlags map[string]bool
}

// doctorFlags are the flags accepted by `devparity doctor`.
var doctorFlags = subcommandSpec{
	name: "doctor",
	boolFlags: map[string]bool{
		"--strict": true,
	},
	valueFlags: map[string]bool{
		"--format": true,
	},
}

// docsVerifyFlags are the flags accepted by `devparity docs verify`.
var docsVerifyFlags = subcommandSpec{
	name: "docs verify",
	boolFlags: map[string]bool{
		"--execute":          true,
		"--trust-repository": true,
		"--container":        true,
		"--allow-network":    true,
	},
	valueFlags: map[string]bool{
		"--format":       true,
		"--env":          true,
		"--timeout":      true,
		"--node-version": true,
	},
}

// allBoolFlags maps every double-dash boolean flag to the subcommand that
// accepts it, so an unknown-for-this-subcommand flag can be reported with a
// pointer to where it does work.
var allBoolFlags = map[string]string{
	"--strict":           "doctor",
	"--execute":          "docs verify",
	"--trust-repository": "docs verify",
	"--container":        "docs verify",
	"--allow-network":    "docs verify",
}

// allValueFlags maps every double-dash value flag to the subcommand that
// accepts it.
var allValueFlags = map[string]string{
	"--format":       "doctor, docs verify",
	"--env":          "docs verify",
	"--timeout":      "docs verify",
	"--node-version": "docs verify",
}

// Usage renders the flags a subcommand accepts using the double-dash style
// the CLI actually parses.
func (spec subcommandSpec) Usage() string {
	names := make([]string, 0, len(spec.boolFlags)+len(spec.valueFlags))
	for name := range spec.boolFlags {
		names = append(names, name)
	}
	for name := range spec.valueFlags {
		names = append(names, name+" <value>")
	}
	sort.Strings(names)
	return fmt.Sprintf("usage: devparity %s [path] %s", spec.name, strings.Join(names, " "))
}

// flagError returns an error for a flag this subcommand does not accept,
// recommending the subcommand that supports it when one exists.
func flagError(spec subcommandSpec, arg string) error {
	if owner, ok := allBoolFlags[arg]; ok && owner != spec.name {
		return fmt.Errorf("flag %s is not supported by %s (it belongs to %s)", arg, spec.name, owner)
	}
	if owner, ok := allValueFlags[arg]; ok && !strings.Contains(", "+owner+", ", ", "+spec.name+", ") {
		return fmt.Errorf("flag %s is not supported by %s (it belongs to %s)", arg, spec.name, owner)
	}
	return fmt.Errorf("unknown flag %s\n%s", arg, spec.Usage())
}

func normalizePathArgs(args []string, spec subcommandSpec) ([]string, string, error) {
	var flags []string
	var path string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name, value, hasValue := strings.Cut(arg, "=")
			if hasValue {
				if !spec.valueFlags[name] || value == "" {
					return nil, "", fmt.Errorf("invalid flag %q\n%s", arg, spec.Usage())
				}
				flags = append(flags, name, value)
				continue
			}
			if spec.valueFlags[arg] {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
					return nil, "", fmt.Errorf("flag %s requires a value\n%s", arg, spec.Usage())
				}
				flags = append(flags, arg, args[i+1])
				i++
				continue
			}
			if !spec.boolFlags[arg] {
				return nil, "", flagError(spec, arg)
			}
			flags = append(flags, arg)
			continue
		}
		if path != "" {
			return nil, "", fmt.Errorf("multiple paths supplied\n%s", spec.Usage())
		}
		path = arg
	}
	return flags, path, nil
}
