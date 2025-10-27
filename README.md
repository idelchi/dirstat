# dirstat

A tool to analyze directory contents and identify space usage.

---

[![GitHub release](https://img.shields.io/github/v/release/idelchi/dirstat)](https://github.com/idelchi/dirstat/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/idelchi/dirstat.svg)](https://pkg.go.dev/github.com/idelchi/dirstat)
[![Go Report Card](https://goreportcard.com/badge/github.com/idelchi/dirstat)](https://goreportcard.com/report/github.com/idelchi/dirstat)
[![Build Status](https://github.com/idelchi/dirstat/actions/workflows/github-actions.yml/badge.svg)](https://github.com/idelchi/dirstat/actions/workflows/github-actions.yml/badge.svg)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`dirstat` recursively scans directories and reports file statistics by extension or directory depth.

## Installation

```sh
curl -sSL https://raw.githubusercontent.com/idelchi/dirstat/refs/heads/main/install.sh | sh -s -- -d ~/.local/bin
```

## Usage

```sh
# Scan current directory
dirstat
```

```sh
# Scan specific path
dirstat /path/to/directory
```

```sh
# Show top 20 largest files
dirstat --top 20
```

```sh
# Analyze directories instead of files
dirstat --dirs
```

```sh
# Filter by extensions (only .go and .md files)
dirstat --ext .go --ext .md
```

```sh
# Exclude extensions (all except .log files)
dirstat --ext '!.log'
```

```sh
# Filter by minimum file size
dirstat --min-size 1MB
```

```sh
# Custom exclusion patterns (regex)
dirstat --exclude '.*test.*' --exclude '.*\.tmp'
```

```sh
# JSON output for scripting
dirstat --output json | jq '.top_files[0]'
```

```sh
# Limit scan depth to 2 levels
dirstat --depth 2
```

```sh
# Analyze directories instead of files
dirstat --dirs
```

```sh
# Show top-level directories only
dirstat --dirs --depth 1
```

```sh
# Analyze subdirectories at depth 2
dirstat --dirs --depth 2
```

## Output

### Table (default)

```text
Top extensions:
  3) .txt:     12 files, 123 KiB (5.1%)
  2) .md:      23 files, 234 KiB (9.8%)
  1) .go:      87 files, 1.9 MiB (80.8%)

Top files:
  3) 'internal/cli/formatter.go'    23 KiB (1.0%)
  2) 'main.go'                      35 KiB (1.5%)
  1) 'internal/dirstat/stats.go'    46 KiB (2.0%)

Stats

Total files:  142
Total size:   2.3 MiB (2453678 bytes)

Elapsed:  123ms
```

## Directory Analysis

Use `--dirs` to aggregate statistics by directory instead of individual files:

```sh
# Show space usage by directories
dirstat --dirs
```

Combine with `--depth` to limit traversal depth:

```sh
# Show top-level directories only
dirstat --dirs --depth 1

# Show directories up to 2 levels deep
dirstat --dirs --depth 2
```

```text
Top directories:
  3) 'cmd'         234 KiB (10.2%)
  2) 'pkg'         456 KiB (19.9%)
  1) 'internal'    1.6 MiB (69.9%)

Stats

Total directories:  3
Total size:         2.3 MiB (2453678 bytes)

Elapsed:  89ms
```

When using `--dirs`, the default exclusion patterns are cleared to avoid
inadvertently filtering out entire directory trees.

> **Note:** `--depth` limits _how deep dirstat scans_, not how far results are aggregated.
>
> For example, `dirstat --dirs --depth 1` only includes files that exist directly inside the first-level directories.
> If a folder (like `.folder/`) contains large files only in deeper subfolders,
> it won't appear in the output because those subfolders are not scanned.
>
> To include all nested data under `.folder/`, use a higher depth (e.g. `--depth 0` for unlimited).

## Shell Integration

Generate shell integration for interactive file removal with `fzf`:

```sh
# Generate integration snippet
dirstat --init
```

Add to your `~/.zshrc`:

```sh
# Load dirstat integration
eval "$(dirstat --init)"
```

The integration adds a `-I` flag that enables interactive mode:

```sh
# Scan and interactively select files to remove
dirstat -I

# Works with all dirstat flags
dirstat -I --ext .go --min-size 1MB
dirstat -I --dirs --depth 2
```

**Interactive mode:**

- Shows top files/directories in `fzf`
- Multi-select with `Tab`
- Preview with `ls -Alh`
- Press `Enter` to generate `rm -rf` commands
- Commands are placed in your shell buffer for review before execution

## Flags

- `--ext`, `-x` — Suffixes to include/exclude (repeatable, use `!` prefix to exclude)
- `--exclude`, `-e` — Regex patterns to exclude (repeatable)
- `--min-size` — Minimum file size (e.g., `1KB`, `10MB`, `1GiB`)
- `--top`, `-t` — Number of top files to display (default: 10)
- `--output`, `-o` — Output format: `json` or `table` (default: `table`)
- `--depth`, `-d` — Maximum traversal depth (0=unlimited, 1=root only, 2=root+1 level, etc.)
- `--dirs` — Analyze directories instead of individual files
- `--debug` — Enable debug output
- `--version`, `-v` — Show version and exit
- `--init`, `-i` — Output shell integration script

**Default exclusions:** `.*\.git/.*`, `.*node_modules/.*`

These defaults are applied unless `--dirs` is used or you provide your own `--exclude` patterns.

## Extension Filtering

Use `!` to exclude specific suffixes:

```sh
# Include everything except .log files
dirstat --ext '!.log'

# Include only source files, exclude tests
dirstat --ext .go --ext '!_test.go'
```

## Use Cases

**Find space-consuming file types**
Identify which extensions take up the most space in a project.

**Clean up build artifacts**
Scan for large build outputs and interactively remove them with `-I`.

**Analyze project structure**
Use `--dirs` to see space distribution across directory hierarchies.

**Automate cleanup scripts**
Use `--output json` to feed statistics into automated cleanup workflows.

## Demo

![Demo](assets/gifs/dirstat.gif)
