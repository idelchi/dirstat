package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/mattn/go-isatty"

	"github.com/idelchi/dirstat/internal/dirstat"
)

func logic(options dirstat.Options) error {
	enableProgress := strings.ToLower(options.Output) != "json" &&
		!options.Debug &&
		isatty.IsTerminal(os.Stderr.Fd())

	ctx := context.Background()

	// Simple progress callback that prints directly to stderr
	var progressHook func(files, bytes int64)

	if enableProgress {
		// Hide cursor for in-place updates; restore on exit.
		fmt.Fprint(os.Stderr, "\033[?25l")
		defer fmt.Fprint(os.Stderr, "\033[?25h")

		progressHook = func(files, bytes int64) {
			msg := fmt.Sprintf("Scanning… %d files, %s",
				files, humanize.IBytes(uint64(bytes))) //nolint:gosec // Bytes is always positive
			fmt.Fprintf(os.Stderr, "\r\033[2K%s\r", msg)
		}
	}

	stats, err := dirstat.Run(ctx, options, progressHook)

	// Clear the status line
	if enableProgress {
		fmt.Fprint(os.Stderr, "\r\033[2K\r")
	}

	if err != nil {
		return err
	}

	switch strings.ToLower(options.Output) {
	case "json":
		return PrintJSON(stats, os.Stdout)
	case "table":
		return PrintTable(stats, os.Stdout)
	default:
		return fmt.Errorf("unknown output format: %s", options.Output)
	}
}
