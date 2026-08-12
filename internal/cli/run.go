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
	fmt.Fprintln(stderr, "usage: devparity <doctor|docs|version>")
	return 2
}
