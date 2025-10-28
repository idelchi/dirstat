package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

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

// Execute runs the CLI with the provided arguments.
//
//nolint:gocognit // Lengthy command setup.
func (c CLI) Execute() error {
	var (
		options    dirstat.Options
		minSizeStr string
		completion string
	)

	defaultExcludes := []string{`.*\.git/.*`, `.*node_modules/.*`}

	defaultTopN := 10

	allowedOutputs := []string{"table", "json"}

	root := &cobra.Command{
		Use:   "dirstat [flags] [path]",
		Short: "Analyze directory contents and report statistics by file extension",
		Long: heredoc.Doc(`
			dirstat analyzes directory contents and reports statistics by file extension.

			Positional Arguments:
			  path                   Directory to analyze. Defaults to current directory if not specified.

			Modes:
			  Default mode analyzes individual files and reports statistics by extension.
			  Use '--dirs' to aggregate by directory instead of individual files.

			The '-I' flag is available if using the integration script for shell usage.
			It will then run an interactive mode where the output of the tool is piped to 'fzf'
		`),
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       c.version,
		RunE: func(cmd *cobra.Command, args []string) error {
			if completion != "" {
				return completions(cmd, completion)
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

			if len(args) == 0 {
				options.Path = "."
			} else {
				options.Path = args[0]
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
			if !cmd.Flags().Lookup("exclude").Changed && options.DirsMode {
				options.Excludes = []string{}
			}

			return logic(options)
		},
	}

	root.Flags().StringSliceVarP(
		&options.Extensions,
		"ext",
		"x",
		[]string{},
		"File suffixes to include (e.g., .go,.md). Use '!' prefix to exclude (e.g., !.log,!_test.go)",
	)
	root.Flags().StringVar(&minSizeStr, "min-size", "0KB", "Minimum file size (e.g., 1KB)")
	root.Flags().IntVarP(&options.TopN, "top", "t", defaultTopN, "Number of top files to display")
	root.Flags().StringVarP(&options.Output, "output", "o", "table", "Output format: json or table")
	root.Flags().StringSliceVarP(&options.Excludes, "exclude", "e", defaultExcludes, "Regex patterns to exclude")
	root.Flags().IntVarP(&options.Depth, "depth", "d", 0, "Maximum traversal depth (0=unlimited)")
	root.Flags().BoolVar(&options.DirsMode, "dirs", false, "Analyze directories instead of individual files")
	root.Flags().BoolVar(&options.Debug, "debug", false, "Enable debug output")
	root.Flags().BoolVarP(&options.Integration, "init", "i", false, "Output init script for shell usage")
	root.Flags().
		StringVar(&completion, "shell-completion", "",
			"Generate shell completion script for specified shell (bash|zsh|fish|powershell)")

	_ = root.Flags().MarkHidden("shell-completion")

	root.Flags().SortFlags = false

	return root.Execute() //nolint:wrapcheck // Error does not need additional wrapping.
}
