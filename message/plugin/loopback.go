package plugin

import (
	"fmt"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// Loopback routes matching output messages back to the engine for re-processing.
// Uses graceful shutdown coordination - loopback outputs are closed after the
// pipeline drains to break cycles.
func Loopback(name string, matcher message.Matcher) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddLoopbackOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("loopback output: %w", err)
		}
		_, err = e.AddLoopbackInput(name, nil, out)
		if err != nil {
			return fmt.Errorf("loopback input: %w", err)
		}
		return nil
	}
}

// ProcessLoopback routes matching output messages back after transformation.
// The handle function returns zero or more messages; return nil to drop.
// Uses graceful shutdown coordination.
func ProcessLoopback(
	name string,
	matcher message.Matcher,
	handle func(*message.Message) []*message.Message,
) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddLoopbackOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("process loopback output: %w", err)
		}

		processed := channel.Process(out, handle)

		_, err = e.AddLoopbackInput(name, nil, processed)
		if err != nil {
			return fmt.Errorf("process loopback input: %w", err)
		}
		return nil
	}
}

// BatchLoopbackConfig configures the BatchLoopback plugin.
type BatchLoopbackConfig struct {
	MaxSize     int           // Flush when batch reaches this size (default: 100).
	MaxDuration time.Duration // Flush after this duration since first item (default: 1s).
}

func (c BatchLoopbackConfig) applyDefaults() BatchLoopbackConfig {
	if c.MaxSize <= 0 {
		c.MaxSize = 100
	}
	if c.MaxDuration <= 0 {
		c.MaxDuration = time.Second
	}
	return c
}

// BatchLoopback batches matching output messages before transformation.
// Batches are sent when MaxSize is reached or MaxDuration elapses.
// The handle function returns zero or more messages; return nil to drop.
// Uses graceful shutdown coordination.
func BatchLoopback(
	name string,
	matcher message.Matcher,
	handle func([]*message.Message) []*message.Message,
	config BatchLoopbackConfig,
) message.Plugin {
	config = config.applyDefaults()
	return func(e *message.Engine) error {
		out, err := e.AddLoopbackOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("batch loopback output: %w", err)
		}

		processed := channel.Batch(out, handle, config.MaxSize, config.MaxDuration)

		_, err = e.AddLoopbackInput(name, nil, processed)
		if err != nil {
			return fmt.Errorf("batch loopback input: %w", err)
		}
		return nil
	}
}

// GroupLoopbackConfig configures the GroupLoopback plugin.
type GroupLoopbackConfig struct {
	MaxSize             int           // Flush group when it reaches this size (default: 100).
	MaxDuration         time.Duration // Flush group after this duration since first item (default: 1s).
	MaxConcurrentGroups int           // Max active groups (default: 0 meaning unlimited).
}

// GroupLoopback groups matching output messages by key before transformation.
// Messages with the same key are batched together until config limits are reached.
// The handle function receives the key and grouped messages; return nil to drop.
// Uses graceful shutdown coordination.
func GroupLoopback[K comparable](
	name string,
	matcher message.Matcher,
	handle func(key K, msgs []*message.Message) []*message.Message,
	keyFunc func(*message.Message) K,
	config GroupLoopbackConfig,
) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddLoopbackOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("group loopback output: %w", err)
		}

		groups := channel.GroupBy(out, keyFunc, channel.GroupByConfig{
			MaxBatchSize:        config.MaxSize,
			MaxDuration:         config.MaxDuration,
			MaxConcurrentGroups: config.MaxConcurrentGroups,
		})
		transformed := channel.Transform(groups, func(g channel.Group[K, *message.Message]) []*message.Message {
			return handle(g.Key, g.Items)
		})
		processed := channel.Flatten(transformed)

		_, err = e.AddLoopbackInput(name, nil, processed)
		if err != nil {
			return fmt.Errorf("group loopback input: %w", err)
		}
		return nil
	}
}
