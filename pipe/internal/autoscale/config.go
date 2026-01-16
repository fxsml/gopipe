package autoscale

import (
	"runtime"
	"time"
)

// Default configuration values.
const (
	DefaultMinWorkers        = 1
	DefaultScaleDownAfter    = 30 * time.Second
	DefaultScaleUpCooldown   = 5 * time.Second
	DefaultScaleDownCooldown = 10 * time.Second
	DefaultCheckInterval     = 1 * time.Second
)

// Config holds the parsed autoscale configuration with defaults applied.
type Config struct {
	MinWorkers        int
	MaxWorkers        int
	ScaleDownAfter    time.Duration
	ScaleUpCooldown   time.Duration
	ScaleDownCooldown time.Duration
	CheckInterval     time.Duration
}

// Parse converts external configuration to internal config with defaults.
func Parse(
	minWorkers, maxWorkers int,
	scaleDownAfter, scaleUpCooldown, scaleDownCooldown, checkInterval time.Duration,
) Config {
	cfg := Config{
		MinWorkers:        minWorkers,
		MaxWorkers:        maxWorkers,
		ScaleDownAfter:    scaleDownAfter,
		ScaleUpCooldown:   scaleUpCooldown,
		ScaleDownCooldown: scaleDownCooldown,
		CheckInterval:     checkInterval,
	}

	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = DefaultMinWorkers
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = runtime.NumCPU()
	}
	if cfg.MinWorkers > cfg.MaxWorkers {
		cfg.MinWorkers = cfg.MaxWorkers
	}
	if cfg.ScaleDownAfter <= 0 {
		cfg.ScaleDownAfter = DefaultScaleDownAfter
	}
	if cfg.ScaleUpCooldown <= 0 {
		cfg.ScaleUpCooldown = DefaultScaleUpCooldown
	}
	if cfg.ScaleDownCooldown <= 0 {
		cfg.ScaleDownCooldown = DefaultScaleDownCooldown
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = DefaultCheckInterval
	}

	return cfg
}
