package acmetel

import "log/slog"

var defaultLogger = slog.Default()

type logger struct {
	*slog.Logger

	stageKind stageKind
	stageName string
	stageID   int
}

func newLogger(stageKind stageKind, stageName string) *logger {
	return &logger{
		Logger: slog.Default(),

		stageKind: stageKind,
		stageName: stageName,

		stageID: 0,
	}
}

func (l *logger) SetStageID(id int) {
	l.stageID = id
}

func (l *logger) getArgs(args []any) []any {
	return append([]any{"stage_kind", l.stageKind, "stage_name", l.stageName, "stage_id", l.stageID}, args...)
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
