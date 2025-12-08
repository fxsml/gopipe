package pubsub

import (
	"context"
	"strings"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// Sender sends messages to topics.
type Sender interface {
	Send(ctx context.Context, topic string, msgs []*message.Message) error
}

// Receiver receives messages from topics.
type Receiver interface {
	Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

// Broker combines sending and receiving capabilities.
type Broker interface {
	Sender
	Receiver
}

// Publisher provides channel-based message publishing with batching and grouping.
type Publisher interface {
	Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

// Subscriber provides channel-based message subscription with polling.
type Subscriber interface {
	Subscribe(ctx context.Context, topic string) <-chan *message.Message
}

type publisher struct {
	publish func(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

func (p *publisher) Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{} {
	return p.publish(ctx, msgs)
}

type subscriber struct {
	subscribe func(ctx context.Context, topic string) <-chan *message.Message
}

func (s *subscriber) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
	return s.subscribe(ctx, topic)
}

// PublisherConfig configures Publisher behavior.
type PublisherConfig struct {
	MaxBatchSize int
	MaxDuration  time.Duration
	Concurrency  int
	Timeout      time.Duration
	Retry        *gopipe.RetryConfig
	Recover      bool
}

// NewPublisher creates a Publisher that batches messages by topic.
func NewPublisher(
	sender Sender,
	route func(msg *message.Message) string,
	config PublisherConfig,
) Publisher {
	proc := gopipe.NewProcessor(func(ctx context.Context, group channel.Group[string, *message.Message]) ([]struct{}, error) {
		return nil, sender.Send(ctx, group.Key, group.Items)
	}, nil)

	opts := []gopipe.Option[channel.Group[string, *message.Message], struct{}]{
		gopipe.WithLogConfig[channel.Group[string, *message.Message], struct{}](gopipe.LogConfig{
			MessageSuccess: "Published messages",
			MessageFailure: "Failed to publish messages",
			MessageCancel:  "Canceled publishing messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[channel.Group[string, *message.Message], struct{}]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[channel.Group[string, *message.Message], struct{}](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[channel.Group[string, *message.Message], struct{}](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[channel.Group[string, *message.Message], struct{}](*config.Retry))
	}

	return &publisher{
		publish: func(ctx context.Context, msgs <-chan *message.Message) <-chan struct{} {
			group := channel.GroupBy(msgs, route, channel.GroupByConfig{
				MaxBatchSize: config.MaxBatchSize,
				MaxDuration:  config.MaxDuration,
			})
			return gopipe.StartProcessor(ctx, group, proc, opts...)
		},
	}
}

// SubscriberConfig configures Subscriber behavior.
type SubscriberConfig struct {
	Concurrency int
	Timeout     time.Duration
	Retry       *gopipe.RetryConfig
	Recover     bool
}

// NewSubscriber creates a Subscriber that polls for messages.
func NewSubscriber(
	receiver Receiver,
	config SubscriberConfig,
) Subscriber {
	opts := []gopipe.Option[struct{}, *message.Message]{
		gopipe.WithLogConfig[struct{}, *message.Message](gopipe.LogConfig{
			MessageSuccess: "Received messages",
			MessageFailure: "Failed to receive messages",
			MessageCancel:  "Canceled receiving messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[struct{}, *message.Message]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[struct{}, *message.Message](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[struct{}, *message.Message](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[struct{}, *message.Message](*config.Retry))
	}

	return &subscriber{
		subscribe: func(ctx context.Context, topic string) <-chan *message.Message {
			return gopipe.NewGenerator(func(ctx context.Context) ([]*message.Message, error) {
				return receiver.Receive(ctx, topic)
			}, opts...).Generate(ctx)
		},
	}
}

// TopicMatcher provides pattern matching for hierarchical topics.
// Topics are "/" separated strings like "orders/created" or "users/profile/updated".
type TopicMatcher struct {
	pattern  string
	segments []string
}

// NewTopicMatcher creates a new topic matcher for the given pattern.
// Patterns support:
//   - Exact match: "orders/created" matches only "orders/created"
//   - Single-level wildcard (+): "orders/+" matches "orders/created", "orders/updated"
//   - Multi-level wildcard (#): "orders/#" matches "orders/created", "orders/created/v2"
func NewTopicMatcher(pattern string) *TopicMatcher {
	return &TopicMatcher{
		pattern:  pattern,
		segments: strings.Split(pattern, "/"),
	}
}

// Matches returns true if the topic matches the pattern.
func (tm *TopicMatcher) Matches(topic string) bool {
	topicSegments := strings.Split(topic, "/")
	return matchSegments(tm.segments, topicSegments)
}

func matchSegments(pattern, topic []string) bool {
	pi, ti := 0, 0

	for pi < len(pattern) && ti < len(topic) {
		switch pattern[pi] {
		case "#":
			// Multi-level wildcard matches everything remaining
			return true
		case "+":
			// Single-level wildcard matches exactly one segment
			pi++
			ti++
		default:
			// Exact match required
			if pattern[pi] != topic[ti] {
				return false
			}
			pi++
			ti++
		}
	}

	// Check if both pattern and topic are exhausted
	if pi == len(pattern) && ti == len(topic) {
		return true
	}

	// Pattern ends with # and we consumed it
	if pi < len(pattern) && pattern[pi] == "#" {
		return true
	}

	return false
}

// SplitTopic splits a topic string into its segments.
func SplitTopic(topic string) []string {
	if topic == "" {
		return nil
	}
	return strings.Split(topic, "/")
}

// JoinTopic joins topic segments into a topic string.
func JoinTopic(segments ...string) string {
	return strings.Join(segments, "/")
}

// ParentTopic returns the parent topic of the given topic.
// Returns empty string if the topic has no parent.
func ParentTopic(topic string) string {
	segments := SplitTopic(topic)
	if len(segments) <= 1 {
		return ""
	}
	return JoinTopic(segments[:len(segments)-1]...)
}

// BaseTopic returns the last segment of the topic.
func BaseTopic(topic string) string {
	segments := SplitTopic(topic)
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}
