// Package engine wraps the tectonic binary: it builds the argument vector from
// a resolved config, runs the compile while capturing output for the analyzer,
// times it, and places the resulting PDF where the user asked.
package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/m3di/texforge/internal/assets"
	"github.com/m3di/texforge/internal/config"
	"github.com/m3di/texforge/internal/logx"
)

type Result struct {
	PDFPath  string
	Duration time.Duration
	Output   string // combined stdout+stderr for the analyzer
	ExitOK   bool
}

type Engine struct {
	Bin string // resolved tectonic path
	Log *logx.Logger
}

// New resolves the tectonic binary or returns a helpful error.
func New(l *logx.Logger) (*Engine, error) {
	bin, ok := assets.FindTectonic()
	if !ok {
		return nil, fmt.Errorf("tectonic engine not found — run the installer (install.sh / install.ps1) or `texforge doctor` for guidance")
	}
	l.Debug("engine resolved", "tectonic", bin)
	return &Engine{Bin: bin, Log: l}, nil
}

// Version returns the tectonic version string.
func (e *Engine) Version() (string, error) {
	out, err := exec.Command(e.Bin, "--version").CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// args assembles the tectonic command line for a single build. We use the
// classic single-file invocation, which every tectonic release supports.
func (e *Engine) args(c config.Config, absInput, outDir string) []string {
	a := []string{
		absInput,
		"--outdir", outDir,
		"--keep-logs",
		"--color", "never",
	}
	if c.Runs > 0 {
		a = append(a, "--reruns", fmt.Sprintf("%d", c.Runs))
	}
	if e.Log.Level() == logx.Debug {
		a = append(a, "--print")
	}
	a = append(a, c.EngineFlags...)
	return a
}

// Build compiles absInput once and returns the result. It streams a live tail
// of engine output to the logger at debug level while buffering everything for
// the analyzer.
func (e *Engine) Build(ctx context.Context, c config.Config, absInput string) (*Result, error) {
	outDir := c.Output
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(filepath.Dir(absInput), c.Output)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	args := e.args(c, absInput, outDir)
	e.Log.Debug("invoking engine", "args", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, e.Bin, args...)
	cmd.Dir = filepath.Dir(absInput)

	// Expose bundled fonts to XeTeX without a system install.
	cmd.Env = os.Environ()
	if c.Fonts && assets.HasFonts() {
		cmd.Env = append(cmd.Env, "OSFONTDIR="+assets.FontDirEnv())
		e.Log.Debug("fonts exposed", "OSFONTDIR", assets.FontDirEnv())
	}

	var buf bytes.Buffer
	tail := &tailWriter{log: e.Log}
	cmd.Stdout = io.MultiWriter(&buf, tail)
	cmd.Stderr = io.MultiWriter(&buf, tail)

	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start)

	// tectonic names the PDF after the input basename.
	base := strings.TrimSuffix(filepath.Base(absInput), filepath.Ext(absInput))
	pdf := filepath.Join(outDir, base+".pdf")

	// Honor a custom output name by renaming the produced PDF.
	if c.Name != "" {
		named := filepath.Join(outDir, c.Name+".pdf")
		if _, err := os.Stat(pdf); err == nil && named != pdf {
			if err := os.Rename(pdf, named); err == nil {
				pdf = named
			}
		}
	}

	res := &Result{
		PDFPath:  pdf,
		Duration: dur,
		Output:   buf.String(),
		ExitOK:   runErr == nil,
	}
	if runErr != nil {
		return res, fmt.Errorf("engine exited with error: %w", runErr)
	}
	if _, err := os.Stat(pdf); err != nil {
		return res, fmt.Errorf("engine reported success but no PDF at %s", pdf)
	}
	return res, nil
}

// tailWriter forwards engine lines to the logger at debug level.
type tailWriter struct {
	log *logx.Logger
	buf []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	for {
		i := bytes.IndexByte(t.buf, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(t.buf[:i]), "\r")
		t.buf = t.buf[i+1:]
		if line != "" {
			t.log.Debug("engine │ " + line)
		}
	}
	return len(p), nil
}
