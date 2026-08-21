package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wenn-id/devparity/internal/analyze"
	"github.com/wenn-id/devparity/internal/report"
)

func runDoctor(args []string, stdout, stderr io.Writer) int {
	flags, path, err := normalizePathArgs(args, map[string]bool{"--format": true})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	set := flag.NewFlagSet("doctor", flag.ContinueOnError)
	set.SetOutput(stderr)
	format := set.String("format", "text", "report format")
	strict := set.Bool("strict", false, "fail when findings exist")
	if err := set.Parse(flags); err != nil {
		return 2
	}
	if path == "" {
		path = "."
	}
	value, err := analyze.Doctor(path, Version)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	switch *format {
	case "text":
		err = report.Text(stdout, value)
	case "json":
		err = report.JSON(stdout, value)
	case "github":
		summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
		if summaryPath == "" {
			fmt.Fprintln(stderr, "GITHUB_STEP_SUMMARY is required for --format github")
			return 2
		}
		// #nosec G304 -- intentional: output path comes from the runtime-controlled GITHUB_STEP_SUMMARY env.
		// #nosec G302 -- GitHub requires the step summary file to be world-readable for Markdown upload.
		file, openErr := os.OpenFile(summaryPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr != nil {
			fmt.Fprintln(stderr, openErr)
			return 2
		}
		err = report.GitHub(file, value)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	default:
		fmt.Fprintf(stderr, "unsupported format %q\n", *format)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *strict && value.Summary.Finding > 0 {
		return 1
	}
	return 0
}
