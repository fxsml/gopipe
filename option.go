package gopipe

import (
	"time"
)

// Option configures behavior of a Pipe.
type Option[In, Out any] func(*config[In, Out])

type config[In, Out any] struct {
	concurrency        int
	buffer             int
	timeout            time.Duration
	contextPropagation bool
	cancel             []CancelFunc[In]

	middleware       []MiddlewareFunc[In, Out]
	metricsCollector []MetricsCollector
	metadataProvider []MiddlewareFunc[In, Out]
	retry            MiddlewareFunc[In, Out]
	recover          bool
	logConfig        *LogConfig
}

func parseConfig[In, Out any](opts []Option[In, Out]) config[In, Out] {
	c := config[In, Out]{
		concurrency:        1,
		buffer:             0,
		timeout:            0,
		contextPropagation: true,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

func (c *config[In, Out]) apply(proc Processor[In, Out]) Processor[In, Out] {
	if c.cancel != nil {
		originalCancel := proc.Cancel
		proc = NewProcessor(proc.Process, func(in In, err error) {
			for i := len(c.cancel) - 1; i >= 0; i-- {
				c.cancel[i](in, err)
			}
			originalCancel(in, err)
		})
	}

	if c.timeout > 0 || !c.contextPropagation {
		proc = useContext[In, Out](c.timeout, c.contextPropagation)(proc)
	}

	if c.logConfig == nil {
		c.logConfig = &defaultLogConfig
	}
	if logger := newMetricsLogger(*c.logConfig); logger != nil {
		c.metricsCollector = append(c.metricsCollector, logger)
	}

	if len(c.metricsCollector) > 1 {
		proc = useMetrics[In, Out](newMetricsDistributor(c.metricsCollector...))(proc)
	}
	if len(c.metricsCollector) == 1 {
		proc = useMetrics[In, Out](c.metricsCollector[0])(proc)
	}

	if c.retry != nil {
		proc = c.retry(proc)
	}

	proc = applyMiddleware(proc, c.middleware...)
	proc = applyMiddleware(proc, c.metadataProvider...)

	if c.recover {
		proc = useRecover[In, Out]()(proc)
	}

	return proc
}
