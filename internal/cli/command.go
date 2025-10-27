package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/dustin/go-humanize"
	"github.com/spf13/pflag"

	"github.com/idelchi/dirstat/internal/dirstat"
	"github.com/idelchi/dirstat/internal/integration"
)

// CLI represents the command-line interface.
type CLI struct {
	version string
}

// New creates a new CLI instance with the given version.
func New(version string) CLI {
	return CLI{version: version}
}

// DefaultExcludes contains the default exclusion patterns.
//
//nolint:gochecknoglobals // Config constant
var DefaultExcludes = []string{`.*\.git/.*`, `.*node_modules/.*`}

func help() {
	//nolint:forbidigo // Help output to console
	fmt.Println(heredoc.Doc(`
		dirstat analyzes directory contents and reports statistics by file extension.

		Usage:

			dirstat [flags] [path]

		Positional Arguments:
		  path                   Directory to analyze. Defaults to current directory if not specified.

		Modes:
		  Default mode analyzes individual files and reports statistics by extension.
		  Use --dirs to aggregate by directory instead of individual files.

		The '-I' flag is available if using the integration script for shell usage.
		It will then run an interactive mode where the output of the tool is piped to 'fzf'

		Flags:
	`))
	pflag.PrintDefaults()
}

// Execute runs the CLI with the provided arguments.
func (c CLI) Execute() error {
	var (
		options    dirstat.Options
		minSizeStr string
	)

	allowedOutputs := []string{"table", "json"}

	pflag.StringSliceVarP(
		&options.Extensions,
		"ext",
		"x",
		[]string{},
		"File suffixes to include (e.g., .go,.md). Use '!' prefix to exclude (e.g., !.log,!_test.go)",
	)
	pflag.StringVar(&minSizeStr, "min-size", "0KB", "Minimum file size (e.g., 1KB)")
	pflag.IntVarP(&options.TopN, "top", "t", 10, "Number of top files to display")
	pflag.StringVarP(&options.Output, "output", "o", "table", "Output format: json or table")
	pflag.StringSliceVarP(&options.Excludes, "exclude", "e", DefaultExcludes, "Regex patterns to exclude")
	pflag.IntVarP(&options.Depth, "depth", "d", 0, "Maximum traversal depth (0=unlimited)")
	pflag.BoolVar(&options.DirsMode, "dirs", false, "Analyze directories instead of individual files")
	pflag.BoolVar(&options.Debug, "debug", false, "Enable debug output")
	pflag.BoolVarP(&options.Version, "version", "v", false, "Show version and exit")
	pflag.BoolVarP(&options.Integration, "init", "i", false, "Output init script for shell usage")

	pflag.CommandLine.SortFlags = false
	pflag.Usage = help
	pflag.Parse()

	if options.Version {
		//nolint:forbidigo // Version output to console
		fmt.Println(c.version)

		return nil
	}

	if options.Integration {
		rendered, err := integration.Render()
		if err != nil {
			return fmt.Errorf("rendering integration script: %w", err)
		}

		//nolint:forbidigo // Integration script output to console
		fmt.Println(rendered)

		return nil
	}

	if !slices.Contains(allowedOutputs, options.Output) {
		return fmt.Errorf("invalid output format %q: must be one of %v", options.Output, allowedOutputs)
	}

	if options.Depth < 0 {
		return errors.New("depth cannot be negative")
	}

	if pflag.NArg() == 0 {
		options.Path = "."
	} else {
		options.Path = pflag.Args()[0]
	}

	// Parse minSize string to bytes
	if minSizeStr != "" {
		size, err := humanize.ParseBytes(minSizeStr)
		if err != nil {
			return fmt.Errorf("invalid min-size: %w", err)
		}

		options.MinSize = int64(size) //nolint:gosec // Size conversion from humanize is safe
	}

	// Clear default excludes if using dirs mode and exclude flag wasn't changed
	if !pflag.Lookup("exclude").Changed && options.DirsMode {
		options.Excludes = []string{}
	}

	return logic(options)
}
