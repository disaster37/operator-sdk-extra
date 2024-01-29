package helper

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap/zapcore"
)

func GetZapLogLevelFromEnv() zapcore.Level {
	switch logLevel, _ := os.LookupEnv("LOG_LEVEL"); strings.ToLower(logLevel) {
	case "trace":
		return zapcore.DebugLevel
	case zapcore.DebugLevel.String():
		return zapcore.DebugLevel
	case zapcore.InfoLevel.String():
		return zapcore.InfoLevel
	case zapcore.WarnLevel.String():
		return zapcore.WarnLevel
	case zapcore.ErrorLevel.String():
		return zapcore.ErrorLevel
	case zapcore.PanicLevel.String():
		return zapcore.PanicLevel
	default:
		return zapcore.InfoLevel
	}
}

func GetZapFormatterFromDev() zapcore.Encoder {
	logFormatter, _ := os.LookupEnv("LOG_FORMATTER")
	logFormatter = strings.ToLower(logFormatter)
	if logFormatter == "json" {
		return zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
	}

	return zapcore.NewConsoleEncoder(zapcore.EncoderConfig{})
}

func GetLogrusLogLevelFromEnv() logrus.Level {
	switch logLevel, _ := os.LookupEnv("LOG_LEVEL"); strings.ToLower(logLevel) {
	case logrus.TraceLevel.String():
		return logrus.TraceLevel
	case logrus.DebugLevel.String():
		return logrus.DebugLevel
	case logrus.InfoLevel.String():
		return logrus.InfoLevel
	case logrus.WarnLevel.String():
		return logrus.WarnLevel
	case logrus.ErrorLevel.String():
		return logrus.ErrorLevel
	case logrus.PanicLevel.String():
		return logrus.PanicLevel
	default:
		return logrus.InfoLevel
	}
}

func GetLogrusFormatterFromEnv() logrus.Formatter {
	logFormatter, _ := os.LookupEnv("LOG_FORMATTER")
	logFormatter = strings.ToLower(logFormatter)
	if logFormatter == "json" {
		return &logrus.JSONFormatter{}
	}

	return &logrus.TextFormatter{}
}
