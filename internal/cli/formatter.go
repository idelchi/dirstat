package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/dustin/go-humanize"

	"github.com/idelchi/dirstat/internal/dirstat"
)

const (
	// TabSpacing is the number of spaces between tabwriter columns.
	TabSpacing = 2
)

// PrintJSON outputs statistics in JSON format.
func PrintJSON(stats *dirstat.Stats, writer io.Writer) error {
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding JSON output: %w", err)
	}

	if _, err := fmt.Fprintln(writer, string(data)); err != nil {
		return err
	}

	return nil
}

// PrintTable outputs statistics in human-readable table format.
//
//nolint:gocognit,varnamelen // Formatting logic requires multiple branches; w is idiomatic for writer
func PrintTable(stats *dirstat.Stats, writer io.Writer) error {
	w := tabwriter.NewWriter(writer, 0, 4, TabSpacing, ' ', 0) //nolint:mnd // Tabwriter configuration

	if !stats.DirectoryMode {
		// Extension statistics
		if _, err := fmt.Fprintln(w, "\nTop extensions:\t\t"); err != nil {
			return err
		}

		extList := make([]string, 0, len(stats.ExtStats))
		for ext := range stats.ExtStats {
			extList = append(extList, ext)
		}

		sort.Slice(extList, func(i, j int) bool {
			return stats.ExtStats[extList[i]].Size < stats.ExtStats[extList[j]].Size
		})

		startIdx := 0

		if len(extList) > stats.TopN {
			startIdx = len(extList) - stats.TopN
		}

		displayList := extList[startIdx:]
		for i, ext := range displayList { //nolint:varnamelen // Standard loop index
			extStat := stats.ExtStats[ext]
			pct := 0.0

			if stats.TotalBytes > 0 {
				pct = 100.0 * float64(extStat.Size) / float64(stats.TotalBytes) //nolint:mnd // Percentage calculation
			}

			if ext == "" {
				ext = "\"\""
			}

			fmt.Fprintf(
				w,
				"  %d) %s:\t%d files, %s (%.1f%%)\n",
				len(displayList)-i,
				ext,
				extStat.Count,
				humanize.IBytes(uint64(extStat.Size)), //nolint:gosec // Size is always positive
				pct,
			)
		}
	}

	// Top files/directories
	if stats.DirectoryMode {
		if _, err := fmt.Fprintln(w, "\nTop directories:\t\t"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "\nTop files:\t\t"); err != nil {
			return err
		}
	}

	for i := range len(stats.TopFiles) { //nolint:varnamelen // Standard loop index
		f := stats.TopFiles[i] //nolint:varnamelen // Common abbreviation for file
		pct := 0.0

		if stats.TotalBytes > 0 {
			pct = 100.0 * float64(f.Size) / float64(stats.TotalBytes) //nolint:mnd // Percentage calculation
		}

		fmt.Fprintf(
			w,
			"  %d) '%s'\t%s (%.1f%%)\n",
			len(stats.TopFiles)-i,
			f.Path,
			humanize.IBytes(uint64(f.Size)), //nolint:gosec // Size is always positive
			pct,
		)
	}

	// Stats summary
	if _, err := fmt.Fprintln(w, "\nStats:\t\t"); err != nil {
		return err
	}

	if stats.DirectoryMode {
		fmt.Fprintf(w, "Total directories:\t%d\n", stats.FileCount)
	} else {
		fmt.Fprintf(w, "Total files:\t%d\n", stats.FileCount)
	}

	fmt.Fprintf(w, "Total size:\t%s (%d bytes)\n",
		humanize.IBytes(uint64(stats.TotalBytes)), stats.TotalBytes) //nolint:gosec // Size is always positive

	fmt.Fprintf(w, "\nElapsed:\t%v\n", stats.Elapsed)

	return w.Flush()
}
