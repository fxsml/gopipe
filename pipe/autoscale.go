package pipe

import (
	"runtime"
	"time"
)

// AutoscaleConfig configures dynamic worker scaling for ProcessPipe.
// When set on Config, enables automatic adjustment of worker count based on load.
//
// Zero values for any field use sensible defaults (see field documentation).
type AutoscaleConfig struct {
	// MinWorkers is the minimum number of workers to maintain.
	// Default: 1
	MinWorkers int

	// MaxWorkers is the maximum number of workers to scale up to.
	// Default: runtime.NumCPU()
	MaxWorkers int

	// ScaleDownAfter is how long a worker must be idle before being stopped.
	// Default: 30s
	ScaleDownAfter time.Duration

	// ScaleUpCooldown is the minimum time between scale-up operations.
	// Prevents thrashing when load spikes briefly.
	// Default: 5s
	ScaleUpCooldown time.Duration

	// ScaleDownCooldown is the minimum time between scale-down operations.
	// Prevents thrashing when load fluctuates.
	// Default: 10s
	ScaleDownCooldown time.Duration

	// CheckInterval is how often the scaler evaluates whether to adjust workers.
	// Default: 1s
	CheckInterval time.Duration
}

var defaultAutoscaleConfig = AutoscaleConfig{
	MinWorkers:        1,
	MaxWorkers:        0, // Will be set to runtime.NumCPU() in parse()
	ScaleDownAfter:    30 * time.Second,
	ScaleUpCooldown:   5 * time.Second,
	ScaleDownCooldown: 10 * time.Second,
	CheckInterval:     1 * time.Second,
}

func (c AutoscaleConfig) parse() AutoscaleConfig {
	if c.MinWorkers <= 0 {
		c.MinWorkers = defaultAutoscaleConfig.MinWorkers
	}
	if c.MaxWorkers <= 0 {
		c.MaxWorkers = runtime.NumCPU()
	}
	if c.MinWorkers > c.MaxWorkers {
		c.MinWorkers = c.MaxWorkers
	}
	if c.ScaleDownAfter <= 0 {
		c.ScaleDownAfter = defaultAutoscaleConfig.ScaleDownAfter
	}
	if c.ScaleUpCooldown <= 0 {
		c.ScaleUpCooldown = defaultAutoscaleConfig.ScaleUpCooldown
	}
	if c.ScaleDownCooldown <= 0 {
		c.ScaleDownCooldown = defaultAutoscaleConfig.ScaleDownCooldown
	}
	if c.CheckInterval <= 0 {
		c.CheckInterval = defaultAutoscaleConfig.CheckInterval
	}
	return c
}
