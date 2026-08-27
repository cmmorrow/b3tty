package src

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
)

// warnWriter is an io.Writer that routes each line through the standard logger
// via Warnf. The http.Server ErrorLog is constructed with flags=0 and no
// prefix so the server writes only the raw message — no timestamp. Warnf then
// calls log.Printf which prepends the timestamp, producing the correct order:
//
//	2026/03/15 02:10:09 [WARN ] http: TLS handshake error …
type warnWriter struct{}

func (warnWriter) Write(p []byte) (int, error) {
	Warnf("%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// NewWarnLogger returns a *log.Logger suitable for http.Server.ErrorLog.
// Internal HTTP/TLS messages are routed through Warnf so the timestamp and
// level label appear in the same order as every other log line.
func NewWarnLogger() *log.Logger {
	return log.New(warnWriter{}, "", 0)
}

// ANSI color escape codes for log level labels and inline formatting.
const (
	ansiReset     = "\033[0m"
	ansiCyan      = "\033[36m"
	ansiYellow    = "\033[33m"
	ansiRed       = "\033[31m"
	ansiMagenta   = "\033[35m"
	ansiBold      = "\033[1m"
	ansiBoldRed   = "\033[1;31m"
	ansiBoldGreen = "\033[1;32m"
)

// useColor is true when stdout is attached to an interactive terminal.
// Colors are suppressed when output is piped or redirected.
var useColor bool

// debugEnabled gates the Debug/Debugf helpers. Set via SetDebug. It's an
// atomic.Bool rather than a plain bool because production only ever writes
// it once at startup (before any request-handling goroutines exist), but
// tests toggle it around handler calls that may still have goroutines from a
// prior test winding down concurrently — a plain bool would race there.
var debugEnabled atomic.Bool

// SetDebug enables or disables debug-level logging.
func SetDebug(enabled bool) {
	debugEnabled.Store(enabled)
}

func init() {
	fi, err := os.Stdout.Stat()
	useColor = err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

// Bold returns s wrapped in bold ANSI codes when color output is enabled,
// and returns s unchanged when output is piped or redirected.
func Bold(s string) string {
	if useColor {
		return ansiBold + s + ansiReset
	}
	return s
}

// BoldGreen returns s in bold green when color output is enabled.
func BoldGreen(s string) string {
	if useColor {
		return ansiBoldGreen + s + ansiReset
	}
	return s
}

func levelLabel(color, label string) string {
	if useColor {
		return color + label + ansiReset
	}
	return label
}

func infoLabel() string  { return levelLabel(ansiCyan, "[INFO ]") }
func warnLabel() string  { return levelLabel(ansiYellow, "[WARN ]") }
func errorLabel() string { return levelLabel(ansiRed, "[ERROR]") }
func fatalLabel() string { return levelLabel(ansiBoldRed, "[FATAL]") }
func debugLabel() string { return levelLabel(ansiMagenta, "[DEBUG]") }

// Infof logs an informational message.
func Infof(format string, args ...any) {
	log.Printf(infoLabel()+" "+format, args...)
}

// Info logs an informational message.
func Info(msg string) {
	log.Println(infoLabel(), msg)
}

// Warnf logs a warning message.
func Warnf(format string, args ...any) {
	log.Printf(warnLabel()+" "+format, args...)
}

// Warn logs a warning message.
func Warn(msg string) {
	log.Println(warnLabel(), msg)
}

// Errorf logs an error message.
func Errorf(format string, args ...any) {
	log.Printf(errorLabel()+" "+format, args...)
}

// Error logs an error message.
func Error(msg string) {
	log.Println(errorLabel(), msg)
}

// Fatalf logs a fatal error message and terminates the process.
func Fatalf(format string, args ...any) {
	log.Fatalf(fatalLabel()+" "+format, args...)
}

// Fatal logs a fatal error message and terminates the process.
func Fatal(args ...any) {
	log.Fatal(append([]any{fatalLabel() + " "}, args...)...)
}

// Debugf logs a debug message. Output is suppressed unless SetDebug(true) has
// been called.
func Debugf(format string, args ...any) {
	if debugEnabled.Load() {
		log.Printf(debugLabel()+" "+format, args...)
	}
}

// Debug logs a debug message. Output is suppressed unless SetDebug(true) has
// been called.
func Debug(msg string) {
	if debugEnabled.Load() {
		log.Println(debugLabel(), msg)
	}
}

// CommandLogger prints colorized messages without timestamps or level label
// prefixes. It is intended for use in CLI subcommands where output should
// read as clean user-facing text rather than structured server log lines.
// Level is indicated by color only: white for Info, yellow for Warn, red for
// Error and Fatal, and magenta for Debug.
type CommandLogger struct {
	l *log.Logger
}

// NewCommandLogger returns a CommandLogger that writes to stdout with no
// timestamp or prefix.
func NewCommandLogger() *CommandLogger {
	return &CommandLogger{l: log.New(os.Stdout, "", 0)}
}

func (c *CommandLogger) colorize(color, s string) string {
	if useColor {
		return color + s + ansiReset
	}
	return s
}

func (c *CommandLogger) Info(msg string) {
	c.l.Println(c.colorize(ansiCyan, msg))
}

func (c *CommandLogger) Infof(format string, args ...any) {
	c.l.Println(c.colorize(ansiCyan, fmt.Sprintf(format, args...)))
}

func (c *CommandLogger) Warn(msg string) {
	c.l.Println(c.colorize(ansiYellow, msg))
}

func (c *CommandLogger) Warnf(format string, args ...any) {
	c.l.Println(c.colorize(ansiYellow, fmt.Sprintf(format, args...)))
}

func (c *CommandLogger) Error(msg string) {
	c.l.Println(c.colorize(ansiRed, msg))
}

func (c *CommandLogger) Errorf(format string, args ...any) {
	c.l.Println(c.colorize(ansiRed, fmt.Sprintf(format, args...)))
}

func (c *CommandLogger) Fatal(msg string) {
	c.l.Fatal(c.colorize(ansiRed, msg))
}

func (c *CommandLogger) Fatalf(format string, args ...any) {
	c.l.Fatal(c.colorize(ansiRed, fmt.Sprintf(format, args...)))
}

func (c *CommandLogger) Debug(msg string) {
	if debugEnabled.Load() {
		c.l.Println(c.colorize(ansiMagenta, msg))
	}
}

func (c *CommandLogger) Debugf(format string, args ...any) {
	if debugEnabled.Load() {
		c.l.Println(c.colorize(ansiMagenta, fmt.Sprintf(format, args...)))
	}
}
