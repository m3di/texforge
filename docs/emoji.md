# Emoji on texforge (tectonic / XeTeX)

**Short version:** emoji render, in **monochrome**. Colour emoji do not work with
this engine, and that is a property of XeTeX, not a texforge bug.

## Why

tectonic is built on **XeTeX**. XeTeX shapes and rasterizes outline glyphs, but
it does **not** support the two colour-emoji font technologies:

- **Bitmap colour** (`CBDT`/`CBLC`) — e.g. *Noto Color Emoji*. XeTeX reserves
  the advance width but draws nothing: you get blank gaps.
- **Layered vector colour** (`COLR`/`CPAL`) — e.g. *Twemoji*, *OpenMoji-Color*.
  Same result: the glyph is skipped.

We verified both empirically against tectonic 0.17 — both produce blank glyphs.

## What texforge does

It bundles **OpenMoji Black**, a monochrome outline emoji font that XeTeX
renders as clean black line-art. Use it as a dedicated face:

```latex
\usepackage{fontspec}
\newfontface\emoji{OpenMoji Black}
\newcommand{\e}[1]{{\emoji\char"#1}}   % \e{1F680} -> 🚀 (monochrome)

Ready to ship \e{1F680}\ \e{2705}
```

`\char"XXXX` takes the Unicode code point in hex. You can also type the emoji
character directly inside the `\emoji{…}` group.

## If you truly need colour emoji

Colour emoji require either a different engine (LuaLaTeX with a colour-font
renderer) or treating emoji as **images**:

- Download the colour PNGs/SVGs from [OpenMoji](https://openmoji.org/) or
  Twemoji and place them with `\includegraphics`.
- Or wrap that in a small macro keyed by code point.

This lives outside XeTeX's text path, so texforge does not do it automatically —
but a document is free to.
