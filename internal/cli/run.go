package cli

import (
	"fmt"
	"io"
)

var Version = "dev"

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, Version)
		return 0
	}
	if len(args) > 0 && args[0] == "doctor" {
		return runDoctor(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "docs" {
		return runDocs(args[1:], stdout, stderr)
	}
	fmt.Fprintln(stderr, "usage: devparity <doctor|docs|version>")
	return 2
}
