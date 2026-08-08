#!/usr/bin/env sh
# texforge installer (macOS / Linux).
#
# Fetches everything needed to render LaTeX with no system TeX install:
#   1. the tectonic engine binary (per OS/arch)     -> ~/.texforge/bin
#   2. the texforge CLI (release binary, or builds)  -> ~/.texforge/bin
#   3. the default font bundle + emoji               -> ~/.texforge/fonts
#   4. installs those fonts by name into your OS font dir
#
# Usage:  ./install.sh            (or: curl -fsSL <raw>/install.sh | sh)
# Env:    TEXFORGE_HOME (default ~/.texforge)   TECTONIC_VERSION   TEXFORGE_REF
set -eu

TECTONIC_VERSION="${TECTONIC_VERSION:-0.17.0}"
TEXFORGE_HOME="${TEXFORGE_HOME:-$HOME/.texforge}"
TEXFORGE_REPO="m3di/texforge"
TEXFORGE_REF="${TEXFORGE_REF:-main}"
BIN="$TEXFORGE_HOME/bin"
FONTS="$TEXFORGE_HOME/fonts"

say()  { printf '\033[1m▸ %s\033[0m\n' "$1"; }
info() { printf '  %s\n' "$1"; }
die()  { printf '\033[31merror: %s\033[0m\n' "$1" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar  >/dev/null 2>&1 || die "tar is required"

mkdir -p "$BIN" "$FONTS" "$TEXFORGE_HOME/dl"

# --- 1. Detect platform -----------------------------------------------------
os="$(uname -s)"; arch="$(uname -m)"
case "$os" in
  Darwin) case "$arch" in
            arm64|aarch64) TECT_TRIPLE="aarch64-apple-darwin" ;;
            x86_64)        TECT_TRIPLE="x86_64-apple-darwin" ;;
            *) die "unsupported macOS arch: $arch" ;;
          esac; TEXFORGE_OS="darwin" ;;
  Linux)  case "$arch" in
            aarch64|arm64) TECT_TRIPLE="aarch64-unknown-linux-musl"; TEXFORGE_ARCH="arm64" ;;
            x86_64)        TECT_TRIPLE="x86_64-unknown-linux-musl";  TEXFORGE_ARCH="amd64" ;;
            *) die "unsupported Linux arch: $arch" ;;
          esac; TEXFORGE_OS="linux" ;;
  *) die "unsupported OS: $os (use install.ps1 on Windows)" ;;
esac
case "$arch" in arm64|aarch64) TEXFORGE_ARCH="arm64" ;; x86_64) TEXFORGE_ARCH="amd64" ;; esac

# --- 2. tectonic engine -----------------------------------------------------
if [ -x "$BIN/tectonic" ] && "$BIN/tectonic" --version >/dev/null 2>&1; then
  say "tectonic already present ($("$BIN/tectonic" --version))"
else
  say "Installing tectonic $TECTONIC_VERSION ($TECT_TRIPLE)"
  url="https://github.com/tectonic-typesetting/tectonic/releases/download/tectonic%40${TECTONIC_VERSION}/tectonic-${TECTONIC_VERSION}-${TECT_TRIPLE}.tar.gz"
  curl -fsSL -o "$TEXFORGE_HOME/dl/tectonic.tar.gz" "$url" || die "download failed: $url"
  tar xzf "$TEXFORGE_HOME/dl/tectonic.tar.gz" -C "$TEXFORGE_HOME/dl"
  mv "$TEXFORGE_HOME/dl/tectonic" "$BIN/tectonic"
  chmod +x "$BIN/tectonic"
  info "tectonic -> $BIN/tectonic"
fi

# --- 3. texforge CLI: release binary, else build from source ----------------
asset="texforge_${TEXFORGE_OS}_${TEXFORGE_ARCH}"
rel="https://github.com/${TEXFORGE_REPO}/releases/latest/download/${asset}"
if [ -x "$BIN/texforge" ] && "$BIN/texforge" version >/dev/null 2>&1; then
  say "texforge already present"
elif curl -fsSL -o "$BIN/texforge" "$rel" 2>/dev/null && [ -s "$BIN/texforge" ]; then
  chmod +x "$BIN/texforge"
  say "Installed texforge CLI (release binary)"
elif command -v go >/dev/null 2>&1; then
  say "No release binary — building texforge from source with Go"
  src="$TEXFORGE_HOME/dl/src"
  if [ -d "$src/.git" ]; then git -C "$src" pull --quiet || true
  else git clone --depth 1 --branch "$TEXFORGE_REF" "https://github.com/${TEXFORGE_REPO}.git" "$src" 2>/dev/null \
       || curl -fsSL "https://github.com/${TEXFORGE_REPO}/archive/refs/heads/${TEXFORGE_REF}.tar.gz" | tar xz -C "$TEXFORGE_HOME/dl" && src="$TEXFORGE_HOME/dl/texforge-${TEXFORGE_REF}"; fi
  ( cd "$src" && go build -ldflags "-X main.Version=${TEXFORGE_REF}" -o "$BIN/texforge" ./cmd/texforge )
  info "texforge -> $BIN/texforge"
else
  die "no prebuilt binary for ${asset} and Go is not installed to build one"
fi

# --- 4. Font bundle ---------------------------------------------------------
say "Fetching font bundle"
GF="https://github.com/google/fonts/raw/main/ofl"
fetch_font() {
  if [ -f "$FONTS/$2" ]; then return 0; fi
  if curl -fsSL -o "$FONTS/$2" "$1/$2"; then info "font: $2"; else info "skip (unavailable): $2"; fi
}
for f in FiraSans-Regular FiraSans-Bold FiraSans-Italic FiraSans-BoldItalic; do
  fetch_font "$GF/firasans" "$f.ttf"; done
for f in IBMPlexSerif-Regular IBMPlexSerif-Bold IBMPlexSerif-Italic; do
  fetch_font "$GF/ibmplexserif" "$f.ttf"; done
for f in IBMPlexMono-Regular IBMPlexMono-Bold; do
  fetch_font "$GF/ibmplexmono" "$f.ttf"; done
# OpenMoji Black — the emoji face XeTeX can render (monochrome). Its source
# filename differs from the installed name, so fetch it explicitly.
if [ ! -f "$FONTS/OpenMoji-Black.ttf" ]; then
  if curl -fsSL -o "$FONTS/OpenMoji-Black.ttf" \
      "https://github.com/hfg-gmuend/openmoji/raw/master/font/OpenMoji-black-glyf/OpenMoji-black-glyf.ttf"; then
    info "font: OpenMoji-Black.ttf"
  else
    info "skip (unavailable): OpenMoji-Black.ttf"
  fi
fi

# --- 5. Register fonts by name ---------------------------------------------
say "Installing fonts into your OS font directory"
"$BIN/texforge" fonts --install || info "(you can rerun 'texforge fonts --install' later)"

# --- Done -------------------------------------------------------------------
say "Done."
case ":$PATH:" in
  *":$BIN:"*) : ;;
  *) info "Add texforge to your PATH:"
     info "  export PATH=\"$BIN:\$PATH\"    # add to ~/.zshrc or ~/.bashrc" ;;
esac
info "Try:  texforge doctor  &&  texforge render examples/report/main.tex"
