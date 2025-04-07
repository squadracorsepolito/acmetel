package internal

import "log/slog"

var defaultLogger = slog.Default()

type Logger struct {
	*slog.Logger

	kind string
	name string
}

func NewLogger(stageKind, stageName string) *Logger {
	return &Logger{
		Logger: slog.Default(),

		kind: stageKind,
		name: stageName,
	}
}

func (l *Logger) getArgs(args []any) []any {
	return append([]any{"kind", l.kind, "name", l.name}, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, l.getArgs(args)...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, l.getArgs(args)...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, l.getArgs(args)...)
}
