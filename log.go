package gopipe

import (
	"errors"
	"log/slog"
	"strings"
)

// LogLevel represents the severity level for logging messages.
type LogLevel string

const (
	// LogLevelDebug is used for detailed information.
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo is used for general information messages.
	LogLevelInfo LogLevel = "info"
	// LogLevelWarn is used for warning conditions.
	LogLevelWarn LogLevel = "warn"
	// LogLevelError is used for error conditions.
	LogLevelError LogLevel = "error"
)

// Logger defines an interface for logging at different severity levels.
type Logger interface {
	// Debug logs a message at debug level.
	Debug(msg string, args ...any)
	// Info logs a message at info level.
	Info(msg string, args ...any)
	// Warn logs a message at warning level.
	Warn(msg string, args ...any)
	// Error logs a message at error level.
	Error(msg string, args ...any)
}

// LogConfig holds configuration for the logger middleware.
// All fields can be customized individually. Defaults from the
// global defaultLoggerConfig are used for any fields not set.
type LogConfig struct {
	// Args are additional arguments to include in all log messages.
	Args []any

	// LevelSuccess is the log level used for successful processing.
	// Defaults to LogLevelDebug.
	LevelSuccess LogLevel
	// LevelCancel is the log level used when processing is canceled.
	// Defaults to LogLevelWarn.
	LevelCancel LogLevel
	// LevelFailure is the log level used when processing fails.
	// Defaults to LogLevelError.
	LevelFailure LogLevel
	// LevelRetry is the log level used when a retry is attempted.
	// Defaults to LogLevelWarn.
	LevelRetry LogLevel

	// MessageSuccess is the message logged on successful processing.
	// Defaults to "GOPIPE: Success".
	MessageSuccess string
	// MessageCancel is the message logged when processing is canceled.
	// Defaults to "GOPIPE: Cancel".
	MessageCancel string
	// MessageFailure is the message logged when processing fails.
	// Defaults to "GOPIPE: Failure".
	MessageFailure string
	// MessageRetry is the message logged when a retry is attempted.
	// Defaults to "GOPIPE: Retry".
	MessageRetry string

	// Disabled disables all logging when set to true.
	Disabled bool
}

// WithLogConfig overrides the default logger configuration for the pipe.
func WithLogConfig[In, Out any](logConfig LogConfig) Option[In, Out] {
	return func(cfg *config[In, Out]) {
		cfg.logConfig = &logConfig
	}
}

// SetDefaultLogConfig sets the default logger configuration for all pipes.
// May be overridden per-pipe using WithLoggerConfig.
func SetDefaultLogConfig(config LogConfig) {
	defaultLogConfig = config.parse()
}

// SetDefaultLogger sets the default logger for all pipes.
// slog.Default() is used by default.
func SetDefaultLogger(l Logger) {
	logger = l
}

var defaultLogConfig = LogConfig{
	LevelSuccess:   LogLevelDebug,
	LevelCancel:    LogLevelWarn,
	LevelFailure:   LogLevelError,
	LevelRetry:     LogLevelWarn,
	MessageSuccess: "GOPIPE: Success",
	MessageCancel:  "GOPIPE: Cancel",
	MessageFailure: "GOPIPE: Failure",
	MessageRetry:   "GOPIPE: Retry",
}

var logger Logger = slog.Default()

func parseLogLevel(level LogLevel) LogLevel {
	level = LogLevel(strings.ToLower(string(level)))
	return level
}

func (c LogConfig) parse() LogConfig {
	c.LevelSuccess = parseLogLevel(c.LevelSuccess)
	if c.LevelSuccess == "" {
		c.LevelSuccess = defaultLogConfig.LevelSuccess
	}
	c.LevelCancel = parseLogLevel(c.LevelCancel)
	if c.LevelCancel == "" {
		c.LevelCancel = defaultLogConfig.LevelCancel
	}
	c.LevelFailure = parseLogLevel(c.LevelFailure)
	if c.LevelFailure == "" {
		c.LevelFailure = defaultLogConfig.LevelFailure
	}
	c.LevelRetry = parseLogLevel(c.LevelRetry)
	if c.LevelRetry == "" {
		c.LevelRetry = defaultLogConfig.LevelRetry
	}
	if c.MessageSuccess == "" {
		c.MessageSuccess = defaultLogConfig.MessageSuccess
	}
	if c.MessageCancel == "" {
		c.MessageCancel = defaultLogConfig.MessageCancel
	}
	if c.MessageFailure == "" {
		c.MessageFailure = defaultLogConfig.MessageFailure
	}
	if c.MessageRetry == "" {
		c.MessageRetry = defaultLogConfig.MessageRetry
	}
	return c
}

func (c *LogConfig) logFunc(level LogLevel, log Logger) func(msg string, args ...any) {
	switch level {
	case LogLevelDebug:
		return log.Debug
	case LogLevelWarn:
		return log.Warn
	case LogLevelError:
		return log.Error
	default:
		return log.Info
	}
}

func appendArgs(args ...[]any) []any {
	l := 0
	for _, a := range args {
		l += len(a)
	}
	result := make([]any, 0, l)
	for _, a := range args {
		result = append(result, a...)
	}
	return result
}

func newMetricsLogger(config LogConfig) MetricsCollector {
	config = config.parse()
	if config.Disabled {
		return nil
	}
	logCancel := config.logFunc(config.LevelCancel, logger)
	logFailure := config.logFunc(config.LevelFailure, logger)
	logSuccess := config.logFunc(config.LevelSuccess, logger)
	logRetry := config.logFunc(config.LevelRetry, logger)
	return func(metrics *Metrics) {
		retryArgs := []any{}
		if metrics.RetryState != nil {
			retryArgs = append(retryArgs,
				"retry_attempts", metrics.RetryState.Attempts,
				"retry_duration", metrics.RetryState.Duration,
			)
		}
		if metrics.Error == nil {
			logSuccess(config.MessageSuccess,
				appendArgs(config.Args, metrics.Metadata.args(), []any{"duration", metrics.Duration}, retryArgs)...)
			return
		}
		if errors.Is(metrics.Error, ErrCancel) {
			logCancel(config.MessageCancel,
				appendArgs(config.Args, metrics.Metadata.args(), []any{"error", metrics.Error})...)
			return
		}
		if metrics.RetryState != nil && metrics.RetryState.Err != nil {
			logFailure(config.MessageFailure,
				appendArgs(config.Args, metrics.Metadata.args(), []any{"error", metrics.Error}, retryArgs)...)
			return
		}
		if metrics.RetryState != nil {
			logRetry(config.MessageRetry,
				appendArgs(config.Args, metrics.Metadata.args(), []any{"error", metrics.Error, "duration", metrics.Duration}, retryArgs)...)
			return
		}
		logFailure(config.MessageFailure,
			appendArgs(config.Args, metrics.Metadata.args(), []any{"error", metrics.Error, "duration", metrics.Duration}, retryArgs)...)
	}
}
