package logging

import (
	"fmt"
	"io"
	"os"
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
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(l.writer, "[DEBUG] %s\n", msg)
}

func (l *Logger) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(l.writer, "%s\n", msg)
}

func (l *Logger) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(l.writer, "ERROR: %s\n", msg)
}
