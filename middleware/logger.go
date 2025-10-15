package middleware

import (
	"context"
	"log/slog"
	"strings"

	"github.com/fxsml/gopipe"
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

// LoggerConfig holds configuration for the logger middleware.
type LoggerConfig struct {
	// Args are additional arguments to include in all log messages.
	Args []any

	// LevelSuccess is the log level used for successful processing.
	LevelSuccess LogLevel
	// LevelCancel is the log level used when processing is canceled.
	LevelCancel LogLevel
	// LevelFailure is the log level used when processing fails.
	LevelFailure LogLevel

	// MessageSuccess is the message logged on successful processing.
	// Defaults to "GOPIPE: Success" if not set.
	MessageSuccess string
	// MessageCancel is the message logged when processing is canceled.
	// Defaults to "GOPIPE: Cancel" if not set.
	MessageCancel string
	// MessageFailure is the message logged when processing fails.
	// Defaults to "GOPIPE: Failure" if not set.
	MessageFailure string
}

func parseLogLevel(level LogLevel) LogLevel {
	level = LogLevel(strings.ToLower(string(level)))
	return level
}

func (c *LoggerConfig) parse() {
	c.LevelSuccess = parseLogLevel(c.LevelSuccess)
	if c.LevelSuccess == "" {
		c.LevelSuccess = LogLevelDebug
	}
	c.LevelCancel = parseLogLevel(c.LevelCancel)
	if c.LevelCancel == "" {
		c.LevelCancel = LogLevelWarn
	}
	c.LevelFailure = parseLogLevel(c.LevelFailure)
	if c.LevelFailure == "" {
		c.LevelFailure = LogLevelError
	}
	if c.MessageSuccess == "" {
		c.MessageSuccess = "GOPIPE: Success"
	}
	if c.MessageCancel == "" {
		c.MessageCancel = "GOPIPE: Cancel"
	}
	if c.MessageFailure == "" {
		c.MessageFailure = "GOPIPE: Failure"
	}
}

func (c *LoggerConfig) logFunc(level LogLevel, log Logger) func(msg string, args ...any) {
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

// UseLogger creates a middleware that logs information about processing results.
// It logs success, failure, or cancellation messages at configured levels,
// and includes any Metadata from the processing context in the log message.
func UseLogger[In, Out any](log Logger, config LoggerConfig) gopipe.MiddlewareFunc[In, Out] {
	config.parse()
	logCancel := config.logFunc(config.LevelCancel, log)
	logFailure := config.logFunc(config.LevelFailure, log)
	logSuccess := config.logFunc(config.LevelSuccess, log)
	return func(next gopipe.Processor[In, Out]) gopipe.Processor[In, Out] {
		return gopipe.NewProcessor(
			func(ctx context.Context, in In) ([]Out, error) {
				val, err := next.Process(ctx, in)
				if err == nil {
					logSuccess(config.MessageSuccess,
						appendArgs(config.Args, MetadataFromContext(ctx).Args())...)
				}
				return val, err
			},
			func(in In, err error) {
				next.Cancel(in, err)
				if gopipe.IsCancel(err) {
					logCancel(config.MessageCancel,
						appendArgs(config.Args, MetadataFromError(err).Args(), []any{"error", err})...)
				} else {
					logFailure(config.MessageFailure,
						appendArgs(config.Args, MetadataFromError(err).Args(), []any{"error", err})...)
				}
			})
	}
}

// UseSlog creates a middleware that logs using the default slog logger.
// Additional arguments can be included in all log messages.
func UseSlog[In, Out any](args ...any) gopipe.MiddlewareFunc[In, Out] {
	return UseLogger[In, Out](slog.Default(), LoggerConfig{
		Args: args,
	})
}
