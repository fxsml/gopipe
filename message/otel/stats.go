package otel

import "github.com/fxsml/gopipe/pipe"

// StatsProvider is implemented by gopipe components that expose a Stats snapshot.
// Satisfied by [message.Router], [message.Merger], [message.Distributor],
// [pipe.ProcessPipe], [pipe.BatchPipe], and [pipe.GeneratePipe].
type StatsProvider interface {
	Stats() pipe.Stats
}
