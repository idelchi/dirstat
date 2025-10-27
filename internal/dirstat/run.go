package dirstat

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"
)

// DefaultProgressInterval is the default interval for progress updates.
const DefaultProgressInterval = 500 * time.Millisecond

// startProgressReporter invokes hook(files, bytes) on each tick until ctx is done.
func startProgressReporter(ctx context.Context, c *collector, hook func(int64, int64), interval time.Duration) {
	if hook == nil {
		return
	}
	if interval <= 0 {
		interval = DefaultProgressInterval
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				files := atomic.LoadInt64(&c.fileCount)
				bytes := atomic.LoadInt64(&c.totalBytes)
				hook(files, bytes)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Run performs directory analysis and returns aggregated statistics.
// It walks the directory tree at opt.Path, filters files based on opt.Extensions
// and opt.Excludes, and collects statistics about file sizes and extensions.
//
// If opt.DirsMode is true, it aggregates statistics by directory instead of
// individual files. If opt.Depth > 0, it limits traversal to the specified depth.
//
// The walk operation can be cancelled via ctx. Progress updates are sent
// to progressHook if provided.
//
//nolint:gocognit,funlen // TODO(Author): Refactor walk logic
func Run(ctx context.Context, opt Options, progressHook func(int64, int64), debug bool) (*Stats, error) {

	if opt.Path == "" {
		opt.Path = "."
	}

	// Normalize to native format to handle both C:/Path and C:\Path inputs
	// filepath.Clean handles both separators and converts to native format
	opt.Path = filepath.Clean(opt.Path)

	// validate path exists and is accessible
	if statInfo, err := os.Stat(opt.Path); err != nil {
		return nil, fmt.Errorf("accessing path %q: %w", opt.Path, err)
	} else if !statInfo.IsDir() {
		return nil, fmt.Errorf("path %q is not a directory", opt.Path)
	}
	// setup extension set for quick lookup
	extInclude := make(map[string]struct{}, len(opt.Extensions))
	extExclude := make(map[string]struct{}, len(opt.Extensions))
	for _, e := range opt.Extensions {
		e = strings.Trim(e, "'\"") // Strip quotes first

		if strings.HasPrefix(e, "!") {
			e = strings.TrimPrefix(e, "!")
			extExclude[e] = struct{}{}
		} else {
			extInclude[e] = struct{}{}
		}
	}

	if opt.TopN <= 0 {
		opt.TopN = 20
	}
	collector := newCollector(opt.TopN, opt.DirsMode)

	// Create context FIRST
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start progress reporter goroutine
	startProgressReporter(ctx, collector, progressHook, opt.ProgressInterval)

	var excludeRegexes []*regexp.Regexp
	for _, p := range opt.Excludes {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("compiling exclusion pattern %q: %w", p, err)
		}
		excludeRegexes = append(excludeRegexes, re)
	}

	start := time.Now()

	// Configure fastwalk
	conf := &fastwalk.Config{
		Follow: false, // Don't follow symlinks
	}

	//nolint:forbidigo // Debug output to console
	if debug {
		fmt.Println()
		fmt.Printf("[debug]: include extensions:\n")
		for ext := range extInclude {
			fmt.Printf("[debug]:   - %s\n", ext)
		}
		fmt.Printf("[debug]: exclude extensions:\n")
		for ext := range extExclude {
			fmt.Printf("[debug]:   - %s\n", ext)
		}
		fmt.Printf("[debug]: exclude regexes:\n")
		for _, re := range excludeRegexes {
			fmt.Printf("[debug]:   - %s\n", re.String())
		}
	}

	// Walk directory with fastwalk (parallel traversal)
	walkErr := fastwalk.Walk(conf, opt.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if debug {
				fmt.Printf("[debug]: error accessing path %s: %v\n", path, err)
			}

			return nil // Silently skip errors
		}

		// Check cancellation periodically
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		// Calculate current depth relative to root
		relPath := strings.TrimPrefix(path, opt.Path)
		relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
		currentDepth := 0
		if relPath != "" {
			currentDepth = strings.Count(relPath, string(filepath.Separator)) + 1
		}

		// Skip if beyond max depth
		if opt.Depth > 0 && currentDepth > opt.Depth {
			if d.IsDir() {
				if debug {
					fmt.Printf("[debug]: skipping directory (beyond depth %d): %s\n", opt.Depth, path)
				}
				return filepath.SkipDir
			}
			if debug {
				fmt.Printf("[debug]: skipping file (beyond depth %d): %s\n", opt.Depth, path)
			}
			return nil
		}

		// Check exclusions for both dirs and files
		if len(excludeRegexes) > 0 {
			for _, re := range excludeRegexes {
				fPath := filepath.ToSlash(path)
				if re.MatchString(fPath) {
					if d.IsDir() {
						if debug {
							fmt.Printf("[debug]: excluding directory: %s\n", fPath)
							fmt.Printf("	 matched regex: %s\n", re.String())
						}
						return filepath.SkipDir // Skip entire directory
					}

					if debug {
						fmt.Printf("[debug]: excluding file: %s\n", fPath)
						fmt.Printf("	 matched regex: %s\n", re.String())
					}

					return nil // Skip this file
				}
			}
		}

		if d.IsDir() {
			return nil
		}

		// Process file directly (no channel, no workers)
		if !d.Type().IsRegular() {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			atomic.AddInt64(&collector.errorCount, 1)
			return nil
		}

		if fileInfo.Size() < opt.MinSize {
			return nil
		}

		if len(extInclude) > 0 {
			matched := false
			for ext := range extInclude {
				if strings.HasSuffix(path, ext) {
					matched = true
					if debug {
						fmt.Printf("[debug]: file matched include pattern: %s (pattern: %s)\n", path, ext)
					}
					break
				}
			}
			if !matched {
				if debug {
					fmt.Printf("[debug]: excluding file (not in include list): %s\n", path)
					fmt.Printf("	 include list: %v\n", extInclude)
				}
				return nil
			}
		}

		if len(extExclude) > 0 {
			for ext := range extExclude {
				if strings.HasSuffix(path, ext) {
					if debug {
						fmt.Printf("[debug]: excluding file (in exclude list): %s\n", path)
						fmt.Printf("	 matched exclude: %s\n", ext)
					}
					return nil
				}
			}
		}

		// Update collector
		if opt.DirsMode {
			// Aggregate by directory (use directory of file, not file itself)
			dirPath := filepath.Dir(path)
			// Make path relative to root for cleaner display
			relDir := strings.TrimPrefix(dirPath, opt.Path)
			relDir = strings.TrimPrefix(relDir, string(filepath.Separator))
			if relDir == "" {
				relDir = "."
			}
			collector.add(relDir, fileInfo.Size(), "DIR:")
		} else {
			ext := filepath.Ext(path)
			collector.add(path, fileInfo.Size(), ext)
		}

		return nil
	})

	if walkErr != nil {
		return nil, walkErr
	}
	stats := collector.finalize()
	stats.Elapsed = time.Since(start)
	return stats, nil
}
