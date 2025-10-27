package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/mattn/go-isatty"

	"github.com/idelchi/dirstat/internal/dirstat"
)

func logic(options Options) error {
	enableProgress := strings.ToLower(options.Output) != "json" &&
		!options.Debug &&
		isatty.IsTerminal(os.Stderr.Fd())

	var minSize int64
	if options.MinSize != "" {
		size, err := humanize.ParseBytes(options.MinSize)
		if err != nil {
			return fmt.Errorf("invalid min-size: %w", err)
		}
		minSize = int64(size)
	}

	dirstatOpts := dirstat.Options{
		Path:       options.Path[0],
		Extensions: options.Exts,
		Excludes:   options.Excludes,
		MinSize:    minSize,
		TopN:       options.TopN,
		Depth:      options.Depth,
		DirsMode:   options.Dirs,
	}

	ctx := context.Background()

	// --- Simple, flicker-free status line ---
	type prog struct{ files, bytes int64 }
	var (
		progCh chan prog
		doneCh chan struct{}
		last   prog
	)
	if enableProgress {
		// Hide cursor for in-place updates; restore on exit.
		fmt.Fprint(os.Stderr, "\033[?25l")
		defer fmt.Fprint(os.Stderr, "\033[?25h")

		progCh = make(chan prog, 1)
		doneCh = make(chan struct{})
		go func() {
			tick := time.NewTicker(250 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case p := <-progCh:
					last = p
				case <-tick.C:
					msg := fmt.Sprintf("Scanning… %d files, %s",
						last.files, humanize.IBytes(uint64(last.bytes)))
					fmt.Fprintf(os.Stderr, "\r\033[2K%s\r", msg)
				case <-doneCh:
					return
				}
			}
		}()
	}

	var progressHook func(files int64, bytes int64)
	if enableProgress {
		progressHook = func(files, bytes int64) {
			select {
			case progCh <- prog{files, bytes}:
			default:
				// drop; we coalesce to latest
			}
		}
	}

	stats, err := dirstat.Run(ctx, dirstatOpts, progressHook, options.Debug)

	// Clear the status line
	if enableProgress {
		close(doneCh)
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
