package logger

import (
	"fmt"
	"io"
	"log"
	"os"
)

// Logger default logger
type Logger struct {
	*log.Logger
}

// NewDefaultLogger create a default logger
func NewLogger(logger *log.Logger) *Logger {
	return &Logger{logger}
}

// Info info log
func (l *Logger) Info(stackoffset int, format string, v ...any) {
	l.Output(2+stackoffset, fmt.Sprintf("[INFO] "+format, v...))
}

// Warn warning log
func (l *Logger) Warn(stackoffset int, format string, v ...any) {
	l.Output(2+stackoffset, fmt.Sprintf("[WARN] "+format, v...))
}

// Error error log
func (l *Logger) Error(stackoffset int, format string, v ...any) {
	l.Output(2+stackoffset, fmt.Sprintf("[ERROR] "+format, v...))
}

// Panic panic log and cause a panic
func (l *Logger) Panic(stackoffset int, format string, v ...any) {
	s := fmt.Sprintf("[PANIC] "+format, v...)
	l.Output(2+stackoffset, s)
	panic(s)
}

// Fatal fatal log and exit
func (l *Logger) Fatal(stackoffset int, format string, v ...any) {
	l.Output(2+stackoffset, fmt.Sprintf("[FATAL] "+format, v...))
	os.Exit(1)
}

// Debug debug log
func (l *Logger) Debug(stackoffset int, format string, v ...any) {
	l.Output(2+stackoffset, fmt.Sprintf("[DEBUG] "+format, v...))
}

var defaultLogger *Logger

func init() {
	flag := log.LstdFlags | log.Lmicroseconds | log.Lshortfile
	defaultLogger = NewLogger(log.New(os.Stdout, "", flag))
}

// RegisterLogger register a logger
func RegisterLogger(flag int, prefix string, out io.Writer) {
	defaultLogger = NewLogger(log.New(out, prefix, flag))
}

func Default() *Logger {
	return defaultLogger
}

// Info info log
func Info(format string, v ...any) {
	defaultLogger.Info(1, format, v...)
}

// Warn warning log
func Warn(format string, v ...any) {
	defaultLogger.Warn(1, format, v...)
}

// Error error log
func Error(format string, v ...any) {
	defaultLogger.Error(1, format, v...)
}

// Panic panic log and cause a panic
func Panic(format string, v ...any) {
	defaultLogger.Panic(1, format, v...)
}

// Fatal fatal log and exit
func Fatal(format string, v ...any) {
	defaultLogger.Fatal(1, format, v...)
}

// Debug debug log
func Debug(format string, v ...any) {
	defaultLogger.Debug(1, format, v...)
}
