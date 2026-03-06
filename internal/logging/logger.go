package logging

import (
	"fmt"
	"io"
	"os"
	"time"
)

type Logger struct {
	debug  bool
	writer io.Writer
}

func New(debug bool, writer io.Writer) *Logger {
	if writer == nil {
		writer = os.Stderr
	}
	return &Logger{debug: debug, writer: writer}
}

func (l *Logger) Debug(format string, args ...any) {
	if !l.debug {
		return
	}
	l.write("DEBUG", format, args...)
}

func (l *Logger) Info(format string, args ...any) {
	l.write("INFO", format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.write("ERROR", format, args...)
}

func (l *Logger) write(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05")
	fmt.Fprintf(l.writer, "[%s] %s: %s\n", timestamp, level, msg)
}
