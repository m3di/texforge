// Package assets locates texforge's runtime dependencies: the tectonic binary
// and the bundled font directory. It centralizes the "where do things live"
// logic so the engine and CLI agree.
package assets

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// Home returns texforge's data directory (~/.texforge), overridable with
// TEXFORGE_HOME. It does not create it.
func Home() string {
	if h := os.Getenv("TEXFORGE_HOME"); h != "" {
		return h
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".texforge")
	}
	return ".texforge"
}

// BinDir is where install scripts drop the tectonic binary.
func BinDir() string { return filepath.Join(Home(), "bin") }

// FontsDir holds bundled/first-run fonts exposed to the engine via OSFONTDIR.
func FontsDir() string { return filepath.Join(Home(), "fonts") }

// CacheDir is tectonic's package cache location we hint at.
func CacheDir() string { return filepath.Join(Home(), "cache") }

func exeName() string {
	if runtime.GOOS == "windows" {
		return "tectonic.exe"
	}
	return "tectonic"
}

// FindTectonic locates the tectonic binary, in priority order:
//  1. TEXFORGE_TECTONIC (explicit override)
//  2. ~/.texforge/bin/tectonic (what install.sh/ps1 provide)
//  3. a "tectonic" alongside the running texforge binary
//  4. anything named tectonic on PATH
//
// The bool reports whether it was found.
func FindTectonic() (string, bool) {
	if p := os.Getenv("TEXFORGE_TECTONIC"); p != "" {
		if isExec(p) {
			return p, true
		}
	}
	if p := filepath.Join(BinDir(), exeName()); isExec(p) {
		return p, true
	}
	if self, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(self), exeName()); isExec(p) {
			return p, true
		}
	}
	if p := lookPath(exeName()); p != "" {
		return p, true
	}
	return "", false
}

func isExec(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return true
}

// FontDirEnv returns the OSFONTDIR value XeTeX honors to discover bundled fonts
// without a system install. It concatenates the texforge fonts dir with any
// existing OSFONTDIR so we never clobber a user's setting.
func FontDirEnv() string {
	dir := FontsDir()
	if existing := os.Getenv("OSFONTDIR"); existing != "" {
		return dir + string(os.PathListSeparator) + existing
	}
	return dir
}

// OSFontDir returns the per-user font directory the platform's font manager
// scans. Installing bundled fonts here is what makes \setmainfont{Name} resolve
// by family name on macOS (Core Text) and Windows (GDI/DirectWrite), where
// OSFONTDIR is ignored.
func OSFontDir() string {
	switch runtime.GOOS {
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Fonts", "texforge")
		}
	case "windows":
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "Microsoft", "Windows", "Fonts")
		}
	default: // linux and friends
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "fonts", "texforge")
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".local", "share", "fonts", "texforge")
		}
	}
	return FontsDir()
}

// InstallFonts copies every bundled font into the OS user font directory so it
// resolves by family name. Returns the count copied and the destination dir.
// On Linux the caller should run `fc-cache` afterwards.
func InstallFonts() (int, string, error) {
	src := FontsDir()
	dst := OSFontDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, dst, err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, dst, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err == nil {
			n++
		}
	}
	return n, dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// HasFonts reports whether the fonts dir exists and holds at least one file.
func HasFonts() bool {
	entries, err := os.ReadDir(FontsDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			return true
		}
	}
	return false
}
