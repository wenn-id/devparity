package nodecmd

import "strings"

type Command struct {
	Manager   string
	Operation string
	Script    string
	Raw       string
}

func Parse(line string) (Command, bool) {
	if strings.ContainsAny(line, "\r\n") {
		return Command{}, false
	}
	fields := strings.Fields(line)
	if len(fields) == 0 || unsafe(fields) {
		return Command{}, false
	}

	command := Command{Manager: fields[0], Raw: line}
	switch command.Manager {
	case "npm":
		switch {
		case len(fields) == 2 && npmInstallAlias(fields[1]):
			command.Operation = "install"
		case len(fields) == 3 && fields[1] == "run":
			command.Operation, command.Script = "script", fields[2]
		case len(fields) == 2 && (fields[1] == "test" || fields[1] == "start"):
			command.Operation, command.Script = fields[1], fields[1]
		default:
			return Command{}, false
		}
	case "pnpm":
		switch {
		case len(fields) == 3 && fields[1] == "run":
			command.Operation, command.Script = "script", fields[2]
		case len(fields) == 2 && pnpmInstallAlias(fields[1]):
			command.Operation = "install"
		case len(fields) == 2 && fields[1] == "add":
			return Command{}, false
		case len(fields) == 2 && fields[1] != "run":
			command.Operation, command.Script = fields[1], fields[1]
		default:
			return Command{}, false
		}
	case "yarn":
		switch {
		case len(fields) == 1:
			command.Operation = "install"
		case len(fields) == 2 && fields[1] == "install":
			command.Operation = "install"
		case len(fields) == 2 && fields[1] == "add":
			return Command{}, false
		case len(fields) == 3 && fields[1] == "run":
			command.Operation, command.Script = "script", fields[2]
		case len(fields) == 2 && fields[1] != "run":
			command.Operation, command.Script = fields[1], fields[1]
		default:
			return Command{}, false
		}
	default:
		return Command{}, false
	}
	return command, true
}

func npmInstallAlias(value string) bool {
	switch value {
	case "install", "i", "add", "ci":
		return true
	default:
		return false
	}
}

func pnpmInstallAlias(value string) bool {
	switch value {
	case "install", "i", "ci":
		return true
	default:
		return false
	}
}

func unsafe(fields []string) bool {
	for _, field := range fields {
		if strings.ContainsAny(field, ";&|<>$`()\\'\"") {
			return true
		}
		if strings.Contains(field, "=") {
			return true
		}
	}
	return false
}
