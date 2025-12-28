package message

// InputConfig configures a typed input channel.
// Used with AddInput for typed *Message inputs (internal use, testing).
type InputConfig struct {
	Name    string  // optional, for tracing/metrics
	Matcher Matcher // optional, nil = match all
}

// RawInputConfig configures a raw input channel.
// Used with AddRawInput for *RawMessage inputs (broker integration).
type RawInputConfig struct {
	Name    string  // optional, for tracing/metrics
	Matcher Matcher // optional, nil = match all
}

// OutputConfig configures a typed output channel.
// Used with AddOutput for typed *Message outputs (internal use, testing).
type OutputConfig struct {
	Name    string  // optional, for logging/metrics
	Matcher Matcher // optional, nil = match all (catch-all)
}

// RawOutputConfig configures a raw output channel.
// Used with AddRawOutput for *RawMessage outputs (broker integration).
type RawOutputConfig struct {
	Name    string  // optional, for logging/metrics
	Matcher Matcher // optional, nil = match all (catch-all)
}

// LoopbackConfig configures internal message re-processing.
type LoopbackConfig struct {
	Name    string  // optional, for tracing/metrics
	Matcher Matcher // required, matches output to loop back
}

// HandlerConfig configures a handler registration.
type HandlerConfig struct {
	Name string // required, handler name for logging/metrics
}

// CommandHandlerConfig configures a command handler.
type CommandHandlerConfig struct {
	Source string         // required, CE source attribute
	Naming NamingStrategy // derives CE types for input and output
}
