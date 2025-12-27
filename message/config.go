package message

// InputConfig configures an input channel.
type InputConfig struct {
	Name    string  // optional, for tracing/metrics
	Matcher Matcher // optional, nil = match all
}

// OutputConfig configures an output channel.
type OutputConfig struct {
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
