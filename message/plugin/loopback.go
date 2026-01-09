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

// BatchLoopback batches matching output messages before transformation.
// Batches are sent when maxSize is reached or maxDuration elapses.
// The handle function returns zero or more messages; return nil to drop.
func BatchLoopback(
	name string,
	matcher message.Matcher,
	handle func([]*message.Message) []*message.Message,
	maxSize int,
	maxDuration time.Duration,
) message.Plugin {
	return func(e *message.Engine) error {
		out, err := e.AddOutput(name, matcher)
		if err != nil {
			return fmt.Errorf("batch loopback output: %w", err)
		}

		processed := channel.Batch(out, handle, maxSize, maxDuration)

		_, err = e.AddInput(name, nil, processed)
		if err != nil {
			return fmt.Errorf("batch loopback input: %w", err)
		}
		return nil
	}
}
