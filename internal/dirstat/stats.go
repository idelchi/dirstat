package dirstat

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ExtStat represents statistics for a file extension.
type ExtStat struct {
	// Count is the number of files with this extension.
	Count int `json:"count"`
	// Size is the cumulative size in bytes.
	Size int64 `json:"size"`
}

// FileStat represents a single file path and size.
type FileStat struct {
	// Path is the file or directory path.
	Path string `json:"path"`
	// Size is the size in bytes.
	Size int64 `json:"size"`
}

// Stats holds aggregate statistics for a directory walk.
type Stats struct {
	// FileCount is the total number of files or directories analyzed.
	FileCount int64 `json:"file_count"`
	// TotalBytes is the cumulative size of all analyzed files.
	TotalBytes int64 `json:"total_bytes"`
	// ExtStats maps file extensions to their statistics.
	ExtStats map[string]ExtStat `json:"ext_stats"`
	// TopFiles contains the N largest files or directories.
	TopFiles []FileStat `json:"top_files"`
	// ErrorCount is the number of errors encountered.
	ErrorCount int64 `json:"error_count"`
	// Elapsed is the total time taken for analysis.
	Elapsed time.Duration `json:"elapsed"`
	// DirectoryMode indicates whether analyzing directories instead of files.
	DirectoryMode bool `json:"directory_mode"`
	// TopN is the number of top results tracked.
	TopN int `json:"top_n"`
}

// Options configures directory analysis and CLI behavior.
type Options struct {
	// Path is the directory to analyze.
	Path string
	// Extensions to include (empty = all).
	Extensions []string
	// Excludes contains regex patterns to exclude.
	Excludes []string
	// MinSize is the minimum file size in bytes.
	MinSize int64
	// TopN is the number of top results to track.
	TopN int
	// Depth is the maximum traversal depth (0=unlimited).
	Depth int
	// DirsMode indicates whether to aggregate by directory instead of files.
	DirsMode bool
	// ProgressInterval controls progress callback cadence.
	ProgressInterval time.Duration
	// Debug indicates whether debug output is enabled.
	Debug bool
	// Output represents output format (table or json).
	Output string
	// Version indicates whether to show version and exit.
	Version bool
	// Integration indicates whether to output integration script.
	Integration bool
}

// collector aggregates statistics from concurrent fastwalk callbacks using a mutex.
type collector struct {
	mu            sync.Mutex // Protect concurrent access
	topN          int
	directoryMode bool
	extStats      map[string]ExtStat
	topFiles      []FileStat
	fileCount     int64
	totalBytes    int64
	errorCount    int64
}

// newCollector creates a collector with the requested configuration.
func newCollector(topN int, directoryMode bool) *collector {
	return &collector{
		topN:          topN,
		directoryMode: directoryMode,
		extStats:      make(map[string]ExtStat),
		topFiles:      make([]FileStat, 0),
	}
}

// addError increments the error counter. This operation is protected by a mutex
// since fastwalk calls the callback from multiple goroutines concurrently.
func (c *collector) addError() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errorCount++
}

// add records a file or directory. This operation is protected by a mutex
// since fastwalk calls the callback from multiple goroutines concurrently.
//
// In directory mode (ext == "DIR:"), path is a directory and we accumulate sizes.
// Otherwise path is a file.
func (c *collector) add(path string, size int64, ext string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.totalBytes += size
	isDirectoryMode := ext == "DIR:"

	if isDirectoryMode {
		stat := c.extStats[path]
		if stat.Count == 0 {
			c.fileCount++
		}
		stat.Count++
		stat.Size += size
		c.extStats[path] = stat
	} else {
		c.fileCount++
		stat := c.extStats[ext]
		stat.Count++
		stat.Size += size
		c.extStats[ext] = stat

		// Collect all files, we'll sort and trim later
		c.topFiles = append(c.topFiles, FileStat{Path: path, Size: size})
	}
}

// finalize produces the final Stats from the collected data.
// It extracts the top N files or directories by size and converts paths
// to slash format for cross-platform consistency.
func (c *collector) finalize() *Stats {
	c.mu.Lock()
	defer c.mu.Unlock()

	var extStats map[string]ExtStat
	var topFiles []FileStat
	var fileCount int64

	if c.directoryMode {
		// Build slice of directories by size
		topFiles = make([]FileStat, 0, len(c.extStats))
		for dirPath, stat := range c.extStats {
			topFiles = append(topFiles, FileStat{Path: dirPath, Size: stat.Size})
		}

		// Sort by size (largest first) and trim to top N
		sort.Slice(topFiles, func(i, j int) bool {
			return topFiles[i].Size > topFiles[j].Size
		})
		if len(topFiles) > c.topN {
			topFiles = topFiles[:c.topN]
		}

		// Reverse for display (smallest first, displayed in reverse)
		for i, j := 0, len(topFiles)-1; i < j; i, j = i+1, j-1 {
			topFiles[i], topFiles[j] = topFiles[j], topFiles[i]
		}

		extStats = make(map[string]ExtStat)
		fileCount = int64(len(c.extStats))
	} else {
		extStats = c.extStats

		// Sort by size (largest first) and trim to top N
		sort.Slice(c.topFiles, func(i, j int) bool {
			return c.topFiles[i].Size > c.topFiles[j].Size
		})
		if len(c.topFiles) > c.topN {
			c.topFiles = c.topFiles[:c.topN]
		}

		// Reverse for display (smallest first, displayed in reverse)
		topFiles = make([]FileStat, len(c.topFiles))
		for i := range c.topFiles {
			topFiles[i] = c.topFiles[len(c.topFiles)-1-i]
		}

		fileCount = c.fileCount
	}

	// Convert all paths to slash format for display
	for i := range topFiles {
		topFiles[i].Path = filepath.ToSlash(topFiles[i].Path)
		// Remove leading "./" prefix
		topFiles[i].Path = strings.TrimPrefix(topFiles[i].Path, "./")
	}

	return &Stats{
		FileCount:     fileCount,
		TotalBytes:    c.totalBytes,
		ExtStats:      extStats,
		TopFiles:      topFiles,
		ErrorCount:    c.errorCount,
		DirectoryMode: c.directoryMode,
		TopN:          c.topN,
	}
}
