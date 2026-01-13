package plugin

import (
	"fmt"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// Loopback routes matching output messages back to the engine for re-processing.
func Loopback(name string, matcher message.Matcher) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("loopback output: %w", err)
		}
		_, err = e.AddInput(name, nil, out)
		if err != nil {
			return fmt.Errorf("loopback input: %w", err)
		}
		return nil
	}
}

// ProcessLoopback routes matching output messages back after transformation.
// The handle function returns zero or more messages; return nil to drop.
func ProcessLoopback(
	name string,
	matcher message.Matcher,
	handle func(*message.Message) []*message.Message,
) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("process loopback output: %w", err)
		}

		processed := channel.Process(out, handle)

		_, err = e.AddInput(name, nil, processed)
		if err != nil {
			return fmt.Errorf("process loopback input: %w", err)
		}
		return nil
	}
}

// BatchLoopbackConfig configures the BatchLoopback plugin.
type BatchLoopbackConfig struct {
	MaxSize     int           // Flush when batch reaches this size.
	MaxDuration time.Duration // Flush after this duration since first item.
}

// BatchLoopback batches matching output messages before transformation.
// Batches are sent when MaxSize is reached or MaxDuration elapses.
// The handle function returns zero or more messages; return nil to drop.
func BatchLoopback(
	name string,
	matcher message.Matcher,
	handle func([]*message.Message) []*message.Message,
	config BatchLoopbackConfig,
) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("batch loopback output: %w", err)
		}

		processed := channel.Batch(out, handle, config.MaxSize, config.MaxDuration)

		_, err = e.AddInput(name, nil, processed)
		if err != nil {
			return fmt.Errorf("batch loopback input: %w", err)
		}
		return nil
	}
}

// GroupLoopbackConfig configures the GroupLoopback plugin.
type GroupLoopbackConfig struct {
	MaxSize        int           // Flush group when it reaches this size.
	MaxDuration    time.Duration // Flush group after this duration since first item.
	MaxConcurrentGroups int      // Max active groups; 0 means unlimited.
}

// GroupLoopback groups matching output messages by key before transformation.
// Messages with the same key are batched together until config limits are reached.
// The handle function receives the key and grouped messages; return nil to drop.
func GroupLoopback[K comparable](
	name string,
	matcher message.Matcher,
	handle func(key K, msgs []*message.Message) []*message.Message,
	keyFunc func(*message.Message) K,
	config GroupLoopbackConfig,
) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddOutput(name, matcher)
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

		_, err = e.AddInput(name, nil, processed)
		if err != nil {
			return fmt.Errorf("group loopback input: %w", err)
		}
		return nil
	}
}
