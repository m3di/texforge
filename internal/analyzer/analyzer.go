// Package analyzer turns raw tectonic/XeTeX output into actionable diagnostics.
//
// It is texforge's "self-optimizing" surface: every build's output is scanned
// for known failure and inefficiency signatures, and each match carries a
// plain-language suggestion for how to fix or speed things up.
package analyzer

import (
	"regexp"
	"sort"
	"strings"
)

type Severity int

const (
	SevInfo Severity = iota
	SevWarn
	SevError
)

func (s Severity) String() string {
	switch s {
	case SevError:
		return "error"
	case SevWarn:
		return "warn"
	default:
		return "info"
	}
}

// Finding is one diagnosed issue with a suggested remedy.
type Finding struct {
	Severity   Severity
	Kind       string // stable machine id, e.g. "missing-package"
	Title      string // one-line human summary
	Detail     string // the evidence line(s) from the log
	Suggestion string // what to do about it
	Count      int    // how many times this signature fired
}

type rule struct {
	kind    string
	sev     Severity
	re      *regexp.Regexp
	title   string
	suggest func(m []string) string
	titlef  func(m []string) string // optional dynamic title
}

// Package names that map cleanly from a missing .sty file.
var rules = []rule{
	{
		kind:  "missing-package",
		sev:   SevError,
		re:    regexp.MustCompile(`(?:File|Package) [` + "`" + `']?([\w-]+\.sty)'? not found`),
		title: "A LaTeX package is missing",
		titlef: func(m []string) string {
			return "Missing package: " + m[1]
		},
		suggest: func(m []string) string {
			return "Tectonic normally fetches packages on demand. If this persists, the package may not be in the bundle — check the name for typos, or run `texforge doctor` and retry with network access so the bundle cache can populate."
		},
	},
	{
		kind:  "undefined-control-sequence",
		sev:   SevError,
		re:    regexp.MustCompile(`Undefined control sequence`),
		title: "Undefined control sequence",
		suggest: func(m []string) string {
			return "A command is used before it is defined — usually a missing \\usepackage. The offending macro is shown on the 'l.<n>' line just below in the log."
		},
	},
	{
		kind:  "missing-font",
		sev:   SevError,
		re:    regexp.MustCompile(`(?:The font|Font|fontspec).*?["` + "`" + `']?([\w \-]+)["'].*?(?:cannot be found|not found|Unknown)`),
		title: "A font could not be found",
		titlef: func(m []string) string {
			return "Font not found: " + strings.TrimSpace(m[1])
		},
		suggest: func(m []string) string {
			return "fontspec cannot see this font. Bundle it and run with --fonts (default), or reference it by file with \\setmainfont[Path=...]{file.ttf}. Run `texforge fonts` to see what texforge ships."
		},
	},
	{
		kind:  "bidi-package-order",
		sev:   SevError,
		re:    regexp.MustCompile(`Oops! you have loaded package (\w+) after bidi`),
		title: "A package was loaded after bidi",
		titlef: func(m []string) string {
			return "Loaded after bidi: " + m[1]
		},
		suggest: func(m []string) string {
			return "bidi — pulled in by polyglossia for any RTL language — has to be loaded last. Move \\usepackage{" + m[1] + "}, and every other package, above \\usepackage{polyglossia}. See docs/rtl.md."
		},
	},
	{
		kind:  "script-font-undefined",
		sev:   SevError,
		re:    regexp.MustCompile(`Please define \\(\w+) with \\newfontfamily`),
		title: "No font is set for this script",
		titlef: func(m []string) string {
			return "Script font not defined: \\" + m[1]
		},
		suggest: func(m []string) string {
			return "polyglossia wants a face covering the script: \\newfontfamily\\" + m[1] + "[Script=Arabic]{<Family>} (fonts are keyed by script, not language). This also fires when a language switch happens inside \\sffamily/\\ttfamily — use the language environment there instead. See docs/rtl.md."
		},
	},
	{
		kind:  "language-env-name-clash",
		sev:   SevError,
		re:    regexp.MustCompile(`\\c@\p{Arabic}`),
		title: "Language environment name clashes with a LaTeX command",
		suggest: func(m []string) string {
			return "\\begin{arabic} is being read as LaTeX's \\arabic counter command, so the text after it is parsed as a counter name. polyglossia ships a capitalised environment for exactly this collision: use \\begin{Arabic} ... \\end{Arabic}, or \\textarabic{...}. See docs/rtl.md."
		},
	},
	{
		kind:  "unknown-language-option",
		sev:   SevError,
		re:    regexp.MustCompile(`Package xkeyval Error: ` + "`" + `([\w-]+)' undefined in families ` + "`" + `([\w-]+)'`),
		title: "Unsupported language option",
		titlef: func(m []string) string {
			return "Unknown option '" + m[1] + "' for language '" + m[2] + "'"
		},
		suggest: func(m []string) string {
			return "This polyglossia release does not accept '" + m[1] + "' for " + m[2] + " — the option set differs between languages and releases. Drop it from \\setotherlanguage[...]{" + m[2] + "} and set the behaviour in the document instead."
		},
	},
	{
		kind:  "command-already-defined",
		sev:   SevError,
		re:    regexp.MustCompile(`Command \\(\S+?) already defined`),
		title: "Command already defined",
		titlef: func(m []string) string {
			return "Already defined: \\" + m[1]
		},
		suggest: func(m []string) string {
			return "\\newcommand is claiming a name a class or package already owns — short ones like \\lang, \\en or \\note collide often. Rename it, or use \\renewcommand if replacing it is deliberate."
		},
	},
	{
		kind:  "file-not-found",
		sev:   SevError,
		re:    regexp.MustCompile(`(?:File|Image|Graphics?) [` + "`" + `']?([^']+?)'? not found`),
		title: "A referenced file is missing",
		titlef: func(m []string) string {
			return "File not found: " + m[1]
		},
		suggest: func(m []string) string {
			return "An \\input/\\include/\\includegraphics points at a file that is not there. Check the path is relative to the main .tex and the extension is correct."
		},
	},
	{
		kind:  "undefined-reference",
		sev:   SevWarn,
		re:    regexp.MustCompile(`Reference [` + "`" + `']?([^']+)'? on page .* undefined`),
		title: "Undefined \\ref / \\cite",
		suggest: func(m []string) string {
			return "A cross-reference is unresolved. This clears itself once the build runs enough passes — texforge lets tectonic manage passes automatically, but a stubborn one means the label really is missing."
		},
	},
	{
		kind:  "rerun",
		sev:   SevInfo,
		re:    regexp.MustCompile(`(?i)rerun to get (cross-references|citations|.*?) right|Label\(s\) may have changed`),
		title: "Document asked for another pass",
		suggest: func(m []string) string {
			return "Labels/refs shifted; another pass is needed. Tectonic handles this itself, so no action is required unless the final PDF still shows '??'."
		},
	},
	{
		kind:  "overfull-hbox",
		sev:   SevWarn,
		re:    regexp.MustCompile(`Overfull \\hbox \(([\d.]+)pt too wide\)`),
		title: "Overfull hbox (text runs into the margin)",
		suggest: func(m []string) string {
			return "A line is too wide for its column. For an overhang of a point or two reach for \\setlength{\\emergencystretch}{2em} — microtype alone will not clear it. Otherwise rephrase, insert a discretionary hyphen \\-, or wrap long unbreakable strings (URLs) in \\url{} / \\seqsplit. Unhyphenated scripts (Arabic, Persian, Hebrew, CJK) hit this more often."
		},
	},
	{
		kind:  "underfull-hbox",
		sev:   SevInfo,
		re:    regexp.MustCompile(`Underfull \\hbox \(badness (\d+)\)`),
		title: "Underfull hbox (loose spacing)",
		suggest: func(m []string) string {
			return "Cosmetic: a line was stretched. Usually safe to ignore; \\sloppy or microtype can smooth it."
		},
	},
	{
		kind:  "overfull-vbox",
		sev:   SevInfo,
		re:    regexp.MustCompile(`Overfull \\vbox`),
		title: "Overfull vbox (content overflows the page height)",
		suggest: func(m []string) string {
			return "Content spilled past the page bottom. Often a too-tall figure/table — try [H] placement, \\resizebox, or a manual pagebreak."
		},
	},
	{
		kind:  "multiply-defined-label",
		sev:   SevWarn,
		re:    regexp.MustCompile(`Label [` + "`" + `']?([^']+)'? multiply defined`),
		title: "A label is defined more than once",
		suggest: func(m []string) string {
			return "Two \\label{} share a key, so \\ref is ambiguous. Rename one."
		},
	},
	{
		kind:  "font-shape-substituted",
		sev:   SevInfo,
		re:    regexp.MustCompile(`Font shape .* undefined|Some font shapes were not available`),
		title: "A font shape was substituted",
		suggest: func(m []string) string {
			return "A requested weight/shape (e.g. bold italic) is not in the font; LaTeX picked a near match. Ship the missing shape or accept the substitution."
		},
	},
}

// Analyze scans combined engine output and returns findings, deduped by kind
// and ordered error → warn → info.
func Analyze(output string) []Finding {
	agg := map[string]*Finding{}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		for i := range rules {
			r := &rules[i]
			m := r.re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			f, ok := agg[r.kind]
			if !ok {
				title := r.title
				if r.titlef != nil {
					title = r.titlef(m)
				}
				f = &Finding{
					Severity:   r.sev,
					Kind:       r.kind,
					Title:      title,
					Detail:     strings.TrimSpace(line),
					Suggestion: r.suggest(m),
				}
				agg[r.kind] = f
			}
			f.Count++
		}
	}

	out := make([]Finding, 0, len(agg))
	for _, f := range agg {
		out = append(out, *f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity // error first
		}
		return out[i].Count > out[j].Count
	})
	return out
}

// Summary returns counts by severity for a one-line report.
func Summary(fs []Finding) (errors, warns, infos int) {
	for _, f := range fs {
		switch f.Severity {
		case SevError:
			errors += f.Count
		case SevWarn:
			warns += f.Count
		default:
			infos += f.Count
		}
	}
	return
}
