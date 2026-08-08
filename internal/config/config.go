// Package config resolves texforge's effective settings from three layers,
// lowest precedence first: built-in defaults, a texforge.toml file, then CLI
// flags. It ships a tiny flat-TOML reader so the binary keeps zero
// dependencies.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Input       string   // source .tex
	Output      string   // output directory
	Name        string   // output basename (without .pdf); empty = derive from input
	Verbosity   string   // debug|info|warn|error
	KeepLogs    bool     // write build log next to the PDF
	Watch       bool     // rebuild on change
	CleanAfter  bool     // remove intermediates after a successful build
	Runs        int      // passes; 0 = let tectonic decide
	EngineFlags []string // extra flags passed straight through to tectonic
	Fonts       bool     // expose bundled fonts to the engine (OSFONTDIR)
}

// Defaults are the values used when nothing else says otherwise.
func Defaults() Config {
	return Config{
		Input:     "",
		Output:    "output",
		Name:      "",
		Verbosity: "info",
		KeepLogs:  true,
		Watch:     false,
		Runs:      0,
		Fonts:     true,
	}
}

// Find walks up from dir looking for a texforge.toml. Returns "" if none.
func Find(dir string) string {
	for {
		p := filepath.Join(dir, "texforge.toml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Load reads a texforge.toml (flat subset) onto an existing Config, mutating
// only the keys present in the file.
func Load(path string, c *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			return fmt.Errorf("%s:%d: expected key = value", path, lineNo)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if i := strings.Index(val, " #"); i >= 0 { // strip trailing comment
			val = strings.TrimSpace(val[:i])
		}
		if err := apply(c, key, val); err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	return sc.Err()
}

func apply(c *Config, key, val string) error {
	switch strings.ToLower(key) {
	case "input":
		c.Input = unquote(val)
	case "output":
		c.Output = unquote(val)
	case "name":
		c.Name = unquote(val)
	case "verbosity":
		c.Verbosity = unquote(val)
	case "keep_logs":
		c.KeepLogs = truthy(val)
	case "watch":
		c.Watch = truthy(val)
	case "clean_after":
		c.CleanAfter = truthy(val)
	case "fonts":
		c.Fonts = truthy(val)
	case "runs":
		n, err := strconv.Atoi(unquote(val))
		if err != nil {
			return fmt.Errorf("runs: %w", err)
		}
		c.Runs = n
	case "engine_flags":
		c.EngineFlags = parseArray(val)
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func truthy(s string) bool {
	switch strings.ToLower(unquote(s)) {
	case "true", "yes", "1", "on":
		return true
	}
	return false
}

// parseArray reads a simple ["a", "b"] TOML array of strings.
func parseArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		p := unquote(strings.TrimSpace(part))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
