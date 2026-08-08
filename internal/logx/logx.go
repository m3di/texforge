// Package logx is texforge's small, dependency-free structured logger.
//
// It writes human-readable, level-controlled lines to stderr and (optionally)
// newline-delimited JSON to a file, so a build is both watchable live and
// machine-parseable after the fact.
package logx

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "trace":
		return Debug
	case "info", "":
		return Info
	case "warn", "warning":
		return Warn
	case "error", "err", "quiet":
		return Error
	default:
		return Info
	}
}

func (l Level) String() string {
	switch l {
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Error:
		return "error"
	}
	return "info"
}

// ANSI colors, disabled when output is not a terminal or NO_COLOR is set.
var (
	cReset  = "\033[0m"
	cGray   = "\033[90m"
	cBlue   = "\033[34m"
	cYellow = "\033[33m"
	cRed    = "\033[31m"
	cBold   = "\033[1m"
)

type Logger struct {
	mu       sync.Mutex
	level    Level
	out      io.Writer // human stream (stderr)
	jsonOut  io.Writer // optional ndjson sink
	useColor bool
}

func New(level Level, color bool) *Logger {
	return &Logger{level: level, out: os.Stderr, useColor: color}
}

// AttachJSON tees structured records to w (typically a build log file).
func (l *Logger) AttachJSON(w io.Writer) {
	l.mu.Lock()
	l.jsonOut = w
	l.mu.Unlock()
}

func (l *Logger) SetLevel(lv Level) { l.level = lv }
func (l *Logger) Level() Level      { return l.level }

func (l *Logger) color(c, s string) string {
	if !l.useColor {
		return s
	}
	return c + s + cReset
}

type record struct {
	Time  string                 `json:"time"`
	Level string                 `json:"level"`
	Msg   string                 `json:"msg"`
	Attrs map[string]interface{} `json:"attrs,omitempty"`
}

func (l *Logger) log(lv Level, msg string, attrs map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// ndjson sink always receives every record regardless of console level.
	if l.jsonOut != nil {
		rec := record{
			Time:  time.Now().UTC().Format(time.RFC3339Nano),
			Level: lv.String(),
			Msg:   msg,
			Attrs: attrs,
		}
		if b, err := json.Marshal(rec); err == nil {
			l.jsonOut.Write(b)
			l.jsonOut.Write([]byte("\n"))
		}
	}

	if lv < l.level {
		return
	}

	var tag string
	switch lv {
	case Debug:
		tag = l.color(cGray, "debug")
	case Info:
		tag = l.color(cBlue, "info ")
	case Warn:
		tag = l.color(cYellow, "warn ")
	case Error:
		tag = l.color(cRed, "error")
	}
	line := fmt.Sprintf("%s %s", tag, msg)
	if len(attrs) > 0 {
		var parts []string
		for k, v := range attrs {
			parts = append(parts, l.color(cGray, fmt.Sprintf("%s=%v", k, v)))
		}
		line += " " + strings.Join(parts, " ")
	}
	fmt.Fprintln(l.out, line)
}

func kv(pairs ...interface{}) map[string]interface{} {
	if len(pairs) == 0 {
		return nil
	}
	m := make(map[string]interface{})
	for i := 0; i+1 < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			k = fmt.Sprint(pairs[i])
		}
		m[k] = pairs[i+1]
	}
	return m
}

func (l *Logger) Debug(msg string, kvs ...interface{}) { l.log(Debug, msg, kv(kvs...)) }
func (l *Logger) Info(msg string, kvs ...interface{})  { l.log(Info, msg, kv(kvs...)) }
func (l *Logger) Warn(msg string, kvs ...interface{})  { l.log(Warn, msg, kv(kvs...)) }
func (l *Logger) Error(msg string, kvs ...interface{}) { l.log(Error, msg, kv(kvs...)) }

// Banner prints a bold headline (info level) — used for step boundaries.
func (l *Logger) Banner(msg string) {
	if Info < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.out, l.color(cBold, "▸ "+msg))
}

func init() {
	if os.Getenv("NO_COLOR") != "" {
		cReset, cGray, cBlue, cYellow, cRed, cBold = "", "", "", "", "", ""
	}
}
