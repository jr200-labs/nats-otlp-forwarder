package logging

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Setup initializes the global zap logger with the given level and format.
// Level: disabled, panic, fatal, error, warn, info, debug, trace.
// If humanReadable is true, uses a console encoder.
func Setup(level string, humanReadable bool) {
	zapLevel := parseLevel(level)

	var cfg zap.Config
	if humanReadable {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	logger, err := cfg.Build(zap.AddStacktrace(zapcore.ErrorLevel))
	if err != nil {
		logger = zap.NewNop()
	}

	zap.ReplaceGlobals(logger)
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "disabled":
		return zapcore.FatalLevel + 1
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	case "error":
		return zapcore.ErrorLevel
	case "warn":
		return zapcore.WarnLevel
	case "info":
		return zapcore.InfoLevel
	case "debug", "trace":
		return zapcore.DebugLevel
	default:
		return zapcore.InfoLevel
	}
}
