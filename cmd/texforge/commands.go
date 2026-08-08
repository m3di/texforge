package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/m3di/texforge/internal/analyzer"
	"github.com/m3di/texforge/internal/assets"
	"github.com/m3di/texforge/internal/config"
	"github.com/m3di/texforge/internal/engine"
	"github.com/m3di/texforge/internal/logx"
)

// intermediateExts are auxiliary files safe to remove between builds.
var intermediateExts = map[string]bool{
	".aux": true, ".log": true, ".out": true, ".toc": true, ".lof": true,
	".lot": true, ".fls": true, ".fdb_latexmk": true, ".synctex.gz": true,
	".bbl": true, ".blg": true, ".bcf": true, ".run.xml": true, ".nav": true,
	".snm": true, ".vrb": true, ".xdv": true, ".idx": true, ".ilg": true,
	".ind": true, ".acn": true, ".acr": true, ".alg": true, ".glo": true,
	".gls": true, ".ist": true,
}

// cleanDir removes intermediate files from dir. When includeLogs is false the
// texforge json log is preserved. Returns the count removed.
func cleanDir(dir string, includeLogs bool) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if intermediateExts[ext] {
			if ext == ".log" && !includeLogs && strings.Contains(name, "texforge") {
				continue
			}
			if os.Remove(filepath.Join(dir, name)) == nil {
				removed++
			}
		}
	}
	return removed
}

func cmdClean(args []string) int {
	dir := "output"
	includeLogs := false
	for _, a := range args {
		switch a {
		case "--all", "--logs":
			includeLogs = true
		case "-h", "--help":
			fmt.Println("Usage: texforge clean [dir] [--all]\n  Remove intermediate/aux files (PDFs are kept). --all also removes logs.")
			return 0
		default:
			if !strings.HasPrefix(a, "-") {
				dir = a
			}
		}
	}
	if _, err := os.Stat(dir); err != nil {
		fmt.Fprintf(os.Stderr, "texforge: nothing to clean at %q\n", dir)
		return 0
	}
	n := cleanDir(dir, includeLogs)
	fmt.Printf("cleaned %d intermediate file(s) in %s\n", n, dir)
	return 0
}

func cmdAnalyze(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: texforge analyze <logfile>\n  Read a tectonic/XeTeX log (or texforge.log.jsonl) and suggest fixes.")
		return 0
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "texforge: %v\n", err)
		return 1
	}
	log := logx.New(logx.Info, os.Getenv("NO_COLOR") == "" && isTerminal(os.Stderr))
	findings := analyzer.Analyze(string(data))
	if len(findings) == 0 {
		log.Info("no known issues found in " + args[0])
		return 0
	}
	report(log, findings)
	e, _, _ := analyzer.Summary(findings)
	if e > 0 {
		return 1
	}
	return 0
}

func cmdDoctor(args []string) int {
	log := logx.New(logx.Info, os.Getenv("NO_COLOR") == "" && isTerminal(os.Stderr))
	ok := true

	// Engine
	if bin, found := assets.FindTectonic(); found {
		eng, _ := engine.New(logx.New(logx.Error, false))
		v, _ := eng.Version()
		log.Info("engine: OK", "path", bin, "version", v)
	} else {
		ok = false
		log.Error("engine: tectonic NOT found")
		log.Info("  ↳ run ./install.sh (mac/linux) or install.ps1 (windows), or `brew install tectonic`")
	}

	// Fonts
	if assets.HasFonts() {
		n := countFonts()
		log.Info("fonts: OK", "dir", assets.FontsDir(), "files", n)
	} else {
		log.Warn("fonts: none bundled yet")
		log.Info("  ↳ run ./install.sh to fetch fonts, or drop .ttf/.otf into " + assets.FontsDir())
	}

	// Config
	if p := config.Find(mustGetwd()); p != "" {
		c := config.Defaults()
		if err := config.Load(p, &c); err != nil {
			ok = false
			log.Error("config: invalid", "path", p, "err", err)
		} else {
			log.Info("config: OK", "path", p)
		}
	} else {
		log.Info("config: none (using defaults)")
	}

	// Home dir writability
	home := assets.Home()
	if err := os.MkdirAll(home, 0o755); err != nil {
		ok = false
		log.Error("home: not writable", "dir", home, "err", err)
	} else {
		log.Info("home: OK", "dir", home)
	}

	if ok {
		log.Info("doctor: healthy ✓")
		return 0
	}
	log.Error("doctor: problems found — see suggestions above")
	return 1
}

func cmdFonts(args []string) int {
	log := logx.New(logx.Info, os.Getenv("NO_COLOR") == "" && isTerminal(os.Stderr))

	for _, a := range args {
		switch a {
		case "--install", "-i":
			return fontsInstall(log)
		case "-h", "--help":
			fmt.Println("Usage: texforge fonts [--install]\n  List bundled fonts, or --install them into the OS font dir so\n  \\setmainfont{Name} resolves by family name on any OS.")
			return 0
		}
	}

	dir := assets.FontsDir()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		log.Warn("no bundled fonts", "dir", dir)
		log.Info("  ↳ run ./install.sh to fetch the default set, or copy .ttf/.otf files into " + dir)
		return 0
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".ttf" || ext == ".otf" || ext == ".ttc" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	log.Info("bundled fonts", "dir", dir, "count", len(names))
	for _, n := range names {
		fmt.Println("  " + n)
	}
	log.Info("exposed to the engine via OSFONTDIR; reference by family name with \\setmainfont{...}")
	return 0
}

// fontsInstall copies bundled fonts into the OS user font directory and, on
// Linux, refreshes the fontconfig cache.
func fontsInstall(log *logx.Logger) int {
	if !assets.HasFonts() {
		log.Error("no bundled fonts to install", "dir", assets.FontsDir())
		log.Info("  ↳ run ./install.sh to fetch the default set first")
		return 1
	}
	n, dst, err := assets.InstallFonts()
	if err != nil {
		log.Error("font install failed", "err", err)
		return 1
	}
	log.Info("installed fonts", "count", n, "dir", dst)
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("fc-cache"); err == nil {
			if err := exec.Command("fc-cache", "-f", dst).Run(); err == nil {
				log.Info("refreshed fontconfig cache")
			}
		} else {
			log.Warn("fc-cache not found — you may need to log out/in for fonts to register")
		}
	}
	log.Info("fonts now resolve by family name, e.g. \\setmainfont{Fira Sans}")
	return 0
}

func countFonts() int {
	entries, _ := os.ReadDir(assets.FontsDir())
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// runWatch rebuilds whenever the input or a sibling source file changes. It
// polls mtimes (no external deps) so it works identically on every OS.
func runWatch(eng *engine.Engine, cfg config.Config, absInput string, log *logx.Logger) int {
	log.Banner("watch " + filepath.Base(absInput))
	log.Info("watching for changes — Ctrl-C to stop", "dir", relTo(filepath.Dir(absInput)))

	ctx, stop := signalContext()
	defer stop()

	var lastBuild time.Time
	build := func() {
		runOnce(eng, cfg, absInput, log)
		lastBuild = time.Now()
	}
	build() // initial

	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("watch stopped")
			return 0
		case <-ticker.C:
			if newest := newestSource(filepath.Dir(absInput)); newest.After(lastBuild) {
				log.Info("change detected — rebuilding")
				build()
			}
		}
	}
}

// newestSource returns the newest mtime among LaTeX source files under dir.
func newestSource(dir string) time.Time {
	var newest time.Time
	watchExts := map[string]bool{".tex": true, ".sty": true, ".cls": true, ".bib": true, ".png": true, ".jpg": true, ".pdf": true}
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.Contains(p, string(os.PathSeparator)+"output"+string(os.PathSeparator)) {
			return nil
		}
		if watchExts[strings.ToLower(filepath.Ext(p))] && fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
		return nil
	})
	return newest
}

func signalContext() (context.Context, func()) {
	return contextWithSignals()
}
