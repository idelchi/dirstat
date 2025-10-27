package dirstat

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charlievieth/fastwalk"
)

// DefaultProgressInterval is the default interval for progress updates.
const DefaultProgressInterval = 500 * time.Millisecond

// logger provides conditional debug output.
type logger struct {
	enabled bool
}

// printf prints debug output if logging is enabled.
func (l logger) printf(format string, args ...any) {
	if l.enabled {
		//nolint:forbidigo // Debug output to console
		fmt.Printf(format, args...)
	}
}

// calculateDepth returns the depth of a path relative to the root.
func calculateDepth(path, root string) int {
	relPath := strings.TrimPrefix(path, root)

	relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
	if relPath == "" {
		return 0
	}

	return strings.Count(relPath, string(filepath.Separator)) + 1
}

// shouldExcludeByPattern checks if path matches any exclusion regex.
func shouldExcludeByPattern(path string, patterns []*regexp.Regexp) *regexp.Regexp {
	if len(patterns) == 0 {
		return nil
	}

	fPath := filepath.ToSlash(path)

	for _, re := range patterns {
		if re.MatchString(fPath) {
			return re
		}
	}

	return nil
}

// shouldIncludeByExtension checks if file should be included based on extension filters.
// Returns true if file should be included, false if excluded.
func shouldIncludeByExtension(path string, include, exclude map[string]struct{}) bool {
	// Check excludes first
	for ext := range exclude {
		if strings.HasSuffix(path, ext) {
			return false
		}
	}
	// If no include filter, include all
	if len(include) == 0 {
		return true
	}
	// Check includes
	for ext := range include {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}

// startProgressReporter invokes hook(files, bytes) on each tick until ctx is done.
//
//nolint:varnamelen // c is idiomatic for collector
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
				c.mu.Lock()

				files := c.fileCount
				bytes := c.totalBytes
				c.mu.Unlock()
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
//nolint:gocognit,funlen // Walk callback complexity is inherent to the filtering logic
func Run(ctx context.Context, opt Options, progressHook func(int64, int64)) (*Stats, error) {
	log := logger{enabled: opt.Debug}

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
	for _, e := range opt.Extensions { //nolint:varnamelen // e is standard for element in range
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

	// Create child context to ensure progress reporter cleanup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start progress reporter goroutine
	startProgressReporter(ctx, collector, progressHook, opt.ProgressInterval)

	excludeRegexes := make([]*regexp.Regexp, 0, len(opt.Excludes))

	for _, p := range opt.Excludes {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("compiling exclusion pattern %q: %w", p, err)
		}

		excludeRegexes = append(excludeRegexes, re)
	}

	log.printf("\n")
	log.printf("[debug]: include extensions:\n")

	for ext := range extInclude {
		log.printf("[debug]:   - %s\n", ext)
	}

	log.printf("[debug]: exclude extensions:\n")

	for ext := range extExclude {
		log.printf("[debug]:   - %s\n", ext)
	}

	log.printf("[debug]: exclude regexes:\n")

	for _, re := range excludeRegexes {
		log.printf("[debug]:   - %s\n", re.String())
	}

	start := time.Now()

	// Configure fastwalk
	conf := &fastwalk.Config{
		Follow: false, // Don't follow symlinks
	}

	// Walk directory with fastwalk (parallel traversal)
	//nolint:varnamelen // d is standard for DirEntry
	walkErr := fastwalk.Walk(conf, opt.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.printf("[debug]: error accessing path %s: %v\n", path, err)

			return nil // Silently skip errors
		}

		// Check cancellation periodically
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		// Calculate current depth and check against limit
		currentDepth := calculateDepth(path, opt.Path)
		if opt.Depth > 0 && currentDepth > opt.Depth {
			if d.IsDir() {
				log.printf("[debug]: skipping directory (beyond depth %d): %s\n", opt.Depth, path)

				return filepath.SkipDir
			}

			log.printf("[debug]: skipping file (beyond depth %d): %s\n", opt.Depth, path)

			return nil
		}

		// Check regex exclusion patterns
		if matchedPattern := shouldExcludeByPattern(path, excludeRegexes); matchedPattern != nil {
			fPath := filepath.ToSlash(path)

			if d.IsDir() {
				log.printf("[debug]: excluding directory: %s\n", fPath)
				log.printf("	 matched regex: %s\n", matchedPattern.String())

				return filepath.SkipDir
			}

			log.printf("[debug]: excluding file: %s\n", fPath)
			log.printf("	 matched regex: %s\n", matchedPattern.String())

			return nil
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
			collector.addError()

			return nil //nolint:nilerr // Intentionally skip errors during walk
		}

		if fileInfo.Size() < opt.MinSize {
			return nil
		}

		// Check extension filters
		if !shouldIncludeByExtension(path, extInclude, extExclude) {
			log.printf("[debug]: excluding file (extension filter): %s\n", path)

			return nil
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
