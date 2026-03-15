package src

import (
	"log"
	"os"
	"strings"
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

// debugEnabled gates the Debug/Debugf helpers. Set via SetDebug.
var debugEnabled bool

// SetDebug enables or disables debug-level logging.
func SetDebug(enabled bool) {
	debugEnabled = enabled
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
	if debugEnabled {
		log.Printf(debugLabel()+" "+format, args...)
	}
}

// Debug logs a debug message. Output is suppressed unless SetDebug(true) has
// been called.
func Debug(msg string) {
	if debugEnabled {
		log.Println(debugLabel(), msg)
	}
}
