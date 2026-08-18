// Command texforge is a cross-OS LaTeX→PDF engine: a thin, observable wrapper
// around the tectonic (XeTeX) binary that self-heals missing packages, exposes
// bundled fonts, and reads its own logs to suggest fixes.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/m3di/texforge/internal/analyzer"
	"github.com/m3di/texforge/internal/config"
	"github.com/m3di/texforge/internal/engine"
	"github.com/m3di/texforge/internal/logx"
)

// Version is stamped at build time via -ldflags "-X main.Version=...".
var Version = "dev"

const usage = `texforge — a self-healing LaTeX→PDF engine on tectonic

Usage:
  texforge <command> [flags]

Commands:
  render <file.tex>   Compile a document to PDF (alias: build)
  watch  <file.tex>   Compile and rebuild on every change
  clean  [dir]        Remove intermediate/aux files (keeps PDFs)
  analyze <log>       Read an engine log and suggest fixes
  doctor              Check the engine, fonts and config are healthy
  fonts               List bundled fonts (and where to add more)
  version             Print the texforge and engine versions

Common flags (render/watch):
  -o, --output <dir>    Output directory            (default: output)
  -n, --name <name>     Output PDF basename         (default: input name)
  -v, --verbose         Verbose (debug) logging
  -q, --quiet           Only errors
      --clean           Remove intermediates after a successful build
      --no-fonts        Do not expose bundled fonts to the engine
      --runs <n>        Force exactly n engine passes
      --config <file>   Use this texforge.toml
  --                    Everything after is passed straight to tectonic

Run 'texforge <command> -h' for command help. Config: texforge.toml (see docs).
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "render", "build":
		os.Exit(cmdRender(args, false))
	case "watch":
		os.Exit(cmdRender(args, true))
	case "clean":
		os.Exit(cmdClean(args))
	case "analyze":
		os.Exit(cmdAnalyze(args))
	case "doctor":
		os.Exit(cmdDoctor(args))
	case "fonts":
		os.Exit(cmdFonts(args))
	case "version", "--version", "-V":
		cmdVersion()
		os.Exit(0)
	case "help", "-h", "--help":
		fmt.Print(usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "texforge: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// parseFlags splits render/watch flags from positional args and engine
// passthrough (everything after "--").
type renderFlags struct {
	output     string
	name       string
	verbose    bool
	quiet      bool
	cleanAft   bool
	noFonts    bool
	runs       int
	configArg  string
	positional []string
	passthru   []string
}

func parseRenderFlags(args []string) (renderFlags, error) {
	var f renderFlags
	f.runs = -1
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			f.passthru = append(f.passthru, args[i+1:]...)
			break
		}
		switch a {
		case "-o", "--output":
			i++
			if i >= len(args) {
				return f, fmt.Errorf("%s needs a value", a)
			}
			f.output = args[i]
		case "-n", "--name":
			i++
			if i >= len(args) {
				return f, fmt.Errorf("%s needs a value", a)
			}
			f.name = args[i]
		case "-v", "--verbose":
			f.verbose = true
		case "-q", "--quiet":
			f.quiet = true
		case "--clean":
			f.cleanAft = true
		case "--no-fonts":
			f.noFonts = true
		case "--runs":
			i++
			if i >= len(args) {
				return f, fmt.Errorf("--runs needs a value")
			}
			fmt.Sscanf(args[i], "%d", &f.runs)
		case "--config":
			i++
			if i >= len(args) {
				return f, fmt.Errorf("--config needs a value")
			}
			f.configArg = args[i]
		case "-h", "--help":
			return f, errHelp
		default:
			if strings.HasPrefix(a, "-") {
				return f, fmt.Errorf("unknown flag %q", a)
			}
			f.positional = append(f.positional, a)
		}
	}
	return f, nil
}

var errHelp = fmt.Errorf("help requested")

// resolveConfig layers defaults ← texforge.toml ← flags.
func resolveConfig(f renderFlags, input string) (config.Config, *logx.Logger) {
	c := config.Defaults()

	cfgPath := f.configArg
	if cfgPath == "" {
		start, _ := os.Getwd()
		if input != "" {
			start = filepath.Dir(mustAbs(input))
		}
		cfgPath = config.Find(start)
	}
	loadedFrom := ""
	if cfgPath != "" {
		if err := config.Load(cfgPath, &c); err == nil {
			loadedFrom = cfgPath
		} else {
			fmt.Fprintf(os.Stderr, "warn  config: %v\n", err)
		}
	}

	// Flags win.
	if f.output != "" {
		c.Output = f.output
	}
	if f.name != "" {
		c.Name = f.name
	}
	if f.cleanAft {
		c.CleanAfter = true
	}
	if f.noFonts {
		c.Fonts = false
	}
	if f.runs >= 0 {
		c.Runs = f.runs
	}
	if len(f.passthru) > 0 {
		c.EngineFlags = append(c.EngineFlags, f.passthru...)
	}
	if f.verbose {
		c.Verbosity = "debug"
	}
	if f.quiet {
		c.Verbosity = "error"
	}

	color := os.Getenv("NO_COLOR") == "" && isTerminal(os.Stderr)
	l := logx.New(logx.ParseLevel(c.Verbosity), color)
	if loadedFrom != "" {
		l.Debug("config loaded", "path", loadedFrom)
	}
	return c, l
}

func cmdRender(args []string, watch bool) int {
	f, err := parseRenderFlags(args)
	if err == errHelp {
		fmt.Print(usage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "texforge: %v\n", err)
		return 2
	}

	c := config.Defaults()
	var input string
	if len(f.positional) > 0 {
		input = f.positional[0]
	}

	cfg, log := resolveConfig(f, input)
	if input == "" {
		input = cfg.Input
	}
	if input == "" {
		log.Error("no input file — pass a .tex path or set input in texforge.toml")
		return 2
	}
	absInput := mustAbs(input)
	if _, err := os.Stat(absInput); err != nil {
		log.Error("input not found", "path", absInput)
		return 2
	}
	_ = c

	eng, err := engine.New(log)
	if err != nil {
		log.Error(err.Error())
		return 1
	}

	if watch {
		return runWatch(eng, cfg, absInput, log)
	}
	ok := runOnce(eng, cfg, absInput, log)
	if ok {
		return 0
	}
	return 1
}

// runOnce performs a single build, attaches a build log, times it, and runs the
// analyzer over the output. Returns true on success.
func runOnce(eng *engine.Engine, cfg config.Config, absInput string, log *logx.Logger) bool {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	outDir := cfg.Output
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(filepath.Dir(absInput), cfg.Output)
	}
	os.MkdirAll(outDir, 0o755)

	// Attach a structured build log next to the output.
	var logFile *os.File
	if cfg.KeepLogs {
		p := filepath.Join(outDir, "texforge.log.jsonl")
		if lf, err := os.Create(p); err == nil {
			logFile = lf
			log.AttachJSON(lf)
			defer logFile.Close()
		}
	}

	log.Banner("render " + filepath.Base(absInput))
	log.Info("building", "input", relTo(absInput), "engine", "tectonic")

	res, err := eng.Build(ctx, cfg, absInput)
	if res == nil {
		log.Error("build failed to start", "err", err)
		return false
	}

	findings := analyzer.Analyze(res.Output)
	report(log, findings)

	if err != nil {
		log.Error("build failed", "after", res.Duration.Round(time.Millisecond))
		if len(findings) == 0 {
			// No signature matched — show the tail so the user isn't blind.
			showTail(log, res.Output)
		}
		return false
	}

	log.Info("done",
		"pdf", relTo(res.PDFPath),
		"took", res.Duration.Round(time.Millisecond),
		"size", humanSize(res.PDFPath),
	)

	if cfg.CleanAfter {
		removed := cleanDir(outDir, false)
		log.Info("cleaned intermediates", "removed", removed)
	}
	return true
}

// report prints analyzer findings grouped by severity with suggestions.
func report(log *logx.Logger, fs []analyzer.Finding) {
	if len(fs) == 0 {
		return
	}
	e, w, _ := analyzer.Summary(fs)
	log.Info("analysis", "issues", len(fs), "errors", e, "warnings", w)
	for _, f := range fs {
		msg := f.Title
		if f.Count > 1 {
			msg = fmt.Sprintf("%s (×%d)", f.Title, f.Count)
		}
		switch f.Severity {
		case analyzer.SevError:
			log.Error(msg)
		case analyzer.SevWarn:
			log.Warn(msg)
		default:
			log.Info(msg)
		}
		log.Info("  ↳ " + f.Suggestion)
	}
}

func cmdVersion() {
	fmt.Printf("texforge %s\n", Version)
	if eng, err := engine.New(logx.New(logx.Error, false)); err == nil {
		if v, err := eng.Version(); err == nil {
			fmt.Println(v)
			return
		}
	}
	fmt.Println("tectonic: not found (run the installer)")
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

func relTo(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return p
}

func humanSize(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return "?"
	}
	return fmtBytes(fi.Size())
}

func fmtBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGT"[exp])
}

func showTail(log *logx.Logger, output string) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	n := 15
	if len(lines) < n {
		n = len(lines)
	}
	log.Error("engine output (last lines):")
	for _, l := range lines[len(lines)-n:] {
		if strings.TrimSpace(l) != "" {
			fmt.Fprintln(os.Stderr, "      "+l)
		}
	}
}
