package autoscale

import "time"

// Default configuration values for testing and documentation.
const (
	DefaultMinWorkers        = 1
	DefaultScaleDownAfter    = 30 * time.Second
	DefaultScaleUpCooldown   = 5 * time.Second
	DefaultScaleDownCooldown = 10 * time.Second
	DefaultCheckInterval     = 1 * time.Second
)
