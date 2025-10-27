package dirstat

import (
	"container/heap"
	"path/filepath"
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

// topHeap is a min‑heap of FileStat based on Size. It implements
// heap.Interface.
type topHeap []FileStat

func (h topHeap) Len() int           { return len(h) }
func (h topHeap) Less(i, j int) bool { return h[i].Size < h[j].Size }
func (h topHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *topHeap) Push(x any) {
	*h = append(*h, x.(FileStat))
}

func (h *topHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// newTopHeap creates & initializes a min‑heap for top‑N tracking.
func newTopHeap() *topHeap {
	h := &topHeap{}
	heap.Init(h)
	return h
}

// Options configures directory analysis.
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
}

// collector aggregates statistics from concurrent fastwalk callbacks using a mutex.
type collector struct {
	mu            sync.Mutex // Protect concurrent access
	topN          int
	directoryMode bool
	extStats      map[string]ExtStat
	topFiles      *topHeap
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
		topFiles:      newTopHeap(),
	}
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

		// Update top files heap
		if c.topFiles.Len() < c.topN {
			heap.Push(c.topFiles, FileStat{Path: path, Size: size})
		} else if size > (*c.topFiles)[0].Size {
			heap.Pop(c.topFiles)
			heap.Push(c.topFiles, FileStat{Path: path, Size: size})
		}
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
		// Build heap of directories by size
		dirHeap := newTopHeap()
		for dirPath, stat := range c.extStats {
			if dirHeap.Len() < c.topN {
				heap.Push(dirHeap, FileStat{Path: dirPath, Size: stat.Size})
			} else if stat.Size > (*dirHeap)[0].Size {
				heap.Pop(dirHeap)
				heap.Push(dirHeap, FileStat{Path: dirPath, Size: stat.Size})
			}
		}

		topFiles = make([]FileStat, dirHeap.Len())
		for i := 0; i < len(topFiles); i++ {
			topFiles[i] = heap.Pop(dirHeap).(FileStat)
		}

		extStats = make(map[string]ExtStat)
		fileCount = int64(len(c.extStats))
	} else {
		extStats = c.extStats

		// Extract top files from heap
		topFiles = make([]FileStat, c.topFiles.Len())
		for i := 0; i < len(topFiles); i++ {
			topFiles[i] = heap.Pop(c.topFiles).(FileStat)
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
