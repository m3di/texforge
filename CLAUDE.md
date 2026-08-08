# texforge — notes for AI assistants

A cross-OS **LaTeX → PDF engine**: a small Go CLI wrapping the **tectonic**
(XeTeX) binary. Read this before making changes.

## What it is / isn't

- **Is:** a single static Go binary that shells out to `tectonic`, streams
  structured logs, times builds, and analyzes engine output to suggest fixes.
- **Isn't:** a TeX distribution. There is no pdflatex, no TeX Live, no Docker.
  The engine is tectonic and only tectonic — that's what gives self-healing
  (on-demand package download).

## Conventions

- **Go, stdlib only.** No third-party modules — the whole point is a
  dependency-free binary. If you reach for a dependency, stop and reconsider.
- **Portability first.** Everything must work on macOS, Linux and Windows.
  Prefer `path/filepath`, `runtime.GOOS`, and OS-agnostic logic. No shelling out
  to Unix-only tools from the CLI.
- **The analyzer is the product's soul.** New engine failure modes should become
  new rules in `internal/analyzer/analyzer.go`, each with a concrete
  `Suggestion`. A rule without a remedy isn't worth adding.
- **Fonts resolve by name.** XeTeX uses the OS font DB on macOS/Windows (not
  `OSFONTDIR`), so bundled fonts are installed into the OS user font dir by
  `texforge fonts --install` / `internal/assets.InstallFonts`. Reference fonts by
  family name in templates, never by machine-specific path.
- **Emoji are monochrome by engine limitation** (XeTeX can't do colour fonts).
  Don't "fix" this by bundling a colour emoji font — it renders blank. See
  `docs/emoji.md`.

## Build & test

```sh
go build -o bin/texforge ./cmd/texforge
go vet ./...
./bin/texforge doctor
./bin/texforge render examples/report/main.tex   # smoke test
```

Requires the tectonic binary at `~/.texforge/bin/tectonic`, on `PATH`, or via
`TEXFORGE_TECTONIC`. Run `./install.sh` once to populate `~/.texforge`.

## Layout

| path | what |
|---|---|
| `cmd/texforge/` | CLI: arg parsing, subcommands, the watch loop |
| `internal/engine/` | tectonic invocation, timing, output capture |
| `internal/analyzer/` | log signatures → suggestions (the issue-finder) |
| `internal/config/` | `texforge.toml` reader, defaults, flag merge |
| `internal/logx/` | leveled logger + ndjson sink |
| `internal/assets/` | engine/font discovery, OS-font-dir install |
| `examples/` | demo templates; their rendered PDFs are committed for the README gallery |

## Regenerating the gallery

After changing an example, re-render and refresh its preview:

```sh
texforge render examples/<name>/main.tex
pdftocairo -png -singlefile -r 110 examples/<name>/output/main.pdf docs/previews/<name>
```

## Releases

Tagging `vX.Y.Z` triggers `.github/workflows/release.yml`, which cross-compiles
`texforge_<os>_<arch>` for all platforms and attaches them to the Release.
`install.sh`/`install.ps1` prefer those prebuilt binaries, falling back to
`go build` only if none is found.
