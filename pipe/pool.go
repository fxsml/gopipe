package pipe

import "time"

// PoolConfig configures the worker pool for a Pipe.
//
// Workers serves dual purpose:
//   - Static mode (MaxWorkers <= Workers): fixed worker count
//   - Autoscale mode (MaxWorkers > Workers): minimum worker count
//
// Autoscaling is enabled when MaxWorkers > Workers.
//
//	| Workers | MaxWorkers | Mode      | Result           |
//	|---------|------------|-----------|------------------|
//	| 0       | 0          | static    | 1 worker (default) |
//	| 4       | 0          | static    | 4 workers        |
//	| 4       | 4          | static    | 4 workers        |
//	| 2       | 16         | autoscale | 2–16 workers     |
type PoolConfig struct {
	// Workers sets the worker count (static mode) or minimum workers (autoscale mode).
	// Default: 1
	Workers int

	// MaxWorkers enables autoscaling when > Workers.
	// Workers scale between Workers and MaxWorkers based on backpressure.
	// If <= Workers (including 0), static mode is used with Workers count.
	MaxWorkers int

	// BufferSize sets the output channel buffer size.
	// Default: 0 (unbuffered)
	BufferSize int

	// Autoscale timing — only used when MaxWorkers > Workers.

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

// isAutoscale reports whether autoscaling is enabled.
// Autoscaling is enabled when MaxWorkers > Workers after defaults are applied.
func (c PoolConfig) isAutoscale() bool {
	return c.MaxWorkers > c.Workers
}

func (c PoolConfig) parse() PoolConfig {
	if c.Workers <= 0 {
		c.Workers = 1
	}
	// Zero MaxWorkers means static mode: clamp to Workers
	if c.MaxWorkers <= 0 {
		c.MaxWorkers = c.Workers
	}
	// MinWorkers cannot exceed MaxWorkers
	if c.Workers > c.MaxWorkers {
		c.Workers = c.MaxWorkers
	}
	// Apply autoscale timing defaults only when autoscale is active
	if c.isAutoscale() {
		if c.ScaleDownAfter <= 0 {
			c.ScaleDownAfter = 30 * time.Second
		}
		if c.ScaleUpCooldown <= 0 {
			c.ScaleUpCooldown = 5 * time.Second
		}
		if c.ScaleDownCooldown <= 0 {
			c.ScaleDownCooldown = 10 * time.Second
		}
		if c.CheckInterval <= 0 {
			c.CheckInterval = time.Second
		}
	}
	return c
}
