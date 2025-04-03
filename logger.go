package acmetel

import "log/slog"

var defaultLogger = slog.Default()

type logger struct {
	*slog.Logger

	kind stageKind
	name string
	id   int
}

func newLogger(stageKind stageKind, stageName string) *logger {
	return &logger{
		Logger: slog.Default(),

		kind: stageKind,
		name: stageName,

		id: 0,
	}
}

func (l *logger) SetID(id int) {
	l.id = id
}

func (l *logger) getArgs(args []any) []any {
	return append([]any{"kind", l.kind, "name", l.name, "id", l.id}, args...)
}

func (l *logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, l.getArgs(args)...)
}

func (l *logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, l.getArgs(args)...)
}

func (l *logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, l.getArgs(args)...)
}
