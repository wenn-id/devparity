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
		case len(fields) == 2 && fields[1] == "ci":
			command.Operation = "install"
		case len(fields) == 3 && fields[1] == "run":
			command.Operation, command.Script = "script", fields[2]
		case len(fields) == 2 && (fields[1] == "test" || fields[1] == "start"):
			command.Operation, command.Script = fields[1], fields[1]
		default:
			return Command{}, false
		}
	case "pnpm", "yarn":
		switch {
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
