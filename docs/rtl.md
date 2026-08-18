# Right-to-left text on texforge (tectonic / XeTeX)

**Short version:** RTL works — Arabic, Persian, Hebrew — through `polyglossia`,
which pulls in `bidi`. Three things bite, and only the first two announce
themselves: **package order**, the **script font**, and **Latin runs inside RTL
text**, which fail silently.

## A preamble that works

```latex
\documentclass[11pt,a4paper]{article}

% 1. Every other package loads FIRST — polyglossia pulls in bidi, which must be last.
\usepackage[margin=2.4cm]{geometry}
\usepackage{fontspec}
\usepackage{xcolor}

\setmainfont{IBM Plex Serif}

% 2. polyglossia last.
\usepackage{polyglossia}
\setmainlanguage{english}
\setotherlanguage{farsi}

% 3. A face that covers the SCRIPT (see below).
\newfontfamily\arabicfont[Script=Arabic,Scale=1.28]{<an Arabic-script family>}

% 4. Latin runs inside RTL text.
\newcommand{\ltr}[1]{\textenglish{#1}}

\begin{document}
An English paragraph.

\begin{farsi}
… Persian text, with \ltr{one Latin phrase} kept in a single box …
\end{farsi}
\end{document}
```

## Package order

```
! Package bidi Error: Oops! you have loaded package geometry after bidi package
```

`bidi` insists on being loaded last, and `polyglossia` loads it for you. So
`\usepackage{polyglossia}` goes **after** every other package — `geometry`,
`fancyhdr`, `hyperref`, all of them. The error names whichever package it caught,
so it is easy to chase one import at a time; move them all at once instead.

## The script font

```
! Package polyglossia Error: The current latin font <X> does not contain the "Arabic" script!
(polyglossia)                Please define \arabicfont with \newfontfamily command.
```

Fonts are keyed by **script**, not by language: Persian, Arabic and Urdu all read
`\arabicfont`. Defining `\farsifont` alone still fails with the message above —
`\arabicfont` is the one polyglossia looks for, and a per-language face is an
optional override on top of it.

The same error appears in a second, less obvious situation: a **language switch
inside a font-family group**. `\textfarsi{…}` within `\sffamily` or `\ttfamily`
trips it, because the family in force is not the one carrying the script. Use the
language environment (`\begin{farsi} … \end{farsi}`) in those places.

texforge bundles no Arabic-script or Hebrew face — Fira Sans, IBM Plex and
OpenMoji are Latin. Supply your own: drop the `.ttf`/`.otf` into
`~/.texforge/fonts`, run `texforge fonts --install`, then reference it by family
name like any other font.

## Latin runs inside RTL text — the silent one

Two separate failures hide here, neither of which stops the build.

**Missing glyphs.** Most Arabic-script faces ship no Latin letters and no em
dash. A Latin word inside RTL text then renders as hollow boxes (tofu) with no
warning at all, because the engine found the font, just not the glyph. Wrap those
runs so they are set in the Latin font, left-to-right:

```latex
\newcommand{\ltr}[1]{\textenglish{#1}}
```

If a character comes out as an empty box, that face lacks it — check before
blaming the shaper.

**Reversed phrases.** Two *adjacent* LTR boxes are laid out right-to-left, like
any other pair of RTL items. So this:

```latex
\ltr{Open} \ltr{Source}      % renders as "Source Open"
\ltr{Open Source}            % correct
```

Wrap the **whole phrase in one box**, spaces included. Nothing warns you; the
words simply come out backwards. When generating LaTeX programmatically, match
runs of Latin words *including the spaces between them*, not word by word.

## Digits and language options

polyglossia's per-language options differ between releases, and an unsupported
one is a hard error:

```
! Package xkeyval Error: `numerals' undefined in families `farsi'.
```

Rather than chase which spelling your release accepts, type the digits you
actually want — `۱۲۳` or `123` — directly in the source.

## Justification

Arabic, Persian and Hebrew do not hyphenate, so paragraphs have fewer break
opportunities and lines run loose or spill a point or two into the margin.
`\setlength{\emergencystretch}{2em}` clears the residue that `microtype` cannot.
