// Package integration provides embedded shell integration snippets.
package integration

import (
	"bytes"
	_ "embed"
	"os/exec"
	"path/filepath"
	"text/template"
)

// ZshFzf contains the zsh shell integration script with fzf support.
//
//go:embed zsh-fzf.sh
var ZshFzf string

// Render renders the integration script to replace the zsh path.
func Render() (string, error) {
	// First use LookPath to find zsh binary
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		return "", err
	}

	zsh = filepath.ToSlash(zsh)

	// Then use text/template to substitute the zsh path
	var rendered string
	tmpl, err := template.New("zsh-fzf").Parse(ZshFzf)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"ZSH": zsh,
	}); err != nil {
		return "", err
	}

	rendered = buf.String()

	return rendered, nil
}
