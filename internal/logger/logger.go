package logger

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Logger struct {
	ctx context.Context
}

func NewLogger() *Logger {
	return &Logger{}
}

func (logger *Logger) Start(ctx context.Context) {
	logger.ctx = ctx
}

func (logger *Logger) Info(message string) {
	runtime.LogInfo(logger.ctx, message)
}

func (logger *Logger) Infof(message string, args ...interface{}) {
	runtime.LogInfof(logger.ctx, message, args...)
}
