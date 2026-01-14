// Package cloudevents provides integration between gopipe's [message.Engine]
// and the CloudEvents SDK protocol bindings.
//
// This package wraps CloudEvents [protocol.Receiver] and [protocol.Sender] interfaces,
// bridging their acknowledgment model (Finish) to gopipe's callback-based [message.Acking].
//
// # Usage with Plugins (Recommended)
//
// Use [SubscriberPlugin] and [PublisherPlugin] for simplified registration:
//
//	// Create protocol instances (using NATS as example)
//	natsReceiver, _ := nats_ce.NewConsumer(conn, subject)
//	kafkaSender, _ := kafka_ce.NewSender(brokers, topic)
//
//	// Register with engine using plugins
//	engine.AddPlugin(
//	    cloudevents.SubscriberPlugin(
//	        ctx, "nats-in", nil,
//	        natsReceiver, cloudevents.SubscriberConfig{},
//	    ),
//	    cloudevents.PublisherPlugin(
//	        ctx, "kafka-out", nil,
//	        kafkaSender, cloudevents.PublisherConfig{},
//	    ),
//	)
//
// # Direct Usage
//
// For more control, use [Subscriber] and [Publisher] directly:
//
//	sub := cloudevents.NewSubscriber(natsReceiver, cloudevents.SubscriberConfig{})
//	pub := cloudevents.NewPublisher(kafkaSender, cloudevents.PublisherConfig{})
//
//	inCh, _ := sub.Subscribe(ctx)
//	engine.AddRawInput("nats-in", nil, inCh)
//	outCh, _ := engine.AddRawOutput("kafka-out", nil)
//	pub.Publish(ctx, outCh)
//
// # Acknowledgment Bridge
//
// CloudEvents uses Finish(err) for acknowledgment:
//   - Finish(nil) = ACK (successful processing)
//   - Finish(err) = NACK (failed processing)
//
// This package bridges to gopipe's [message.Acking]:
//   - [message.RawMessage.Ack] calls ceMsg.Finish(nil)
//   - [message.RawMessage.Nack] calls ceMsg.Finish(err)
//
// # Conversion Functions
//
// For manual conversion between CloudEvents and gopipe messages:
//   - [FromCloudEvent] converts a CloudEvents event to [message.RawMessage]
//   - [ToCloudEvent] converts a [message.RawMessage] to a CloudEvents event
//
// # Supported Protocol Bindings
//
// Any CloudEvents SDK protocol binding can be used:
//   - HTTP (net/http)
//   - Kafka (Sarama or Confluent)
//   - AMQP
//   - NATS / NATS JetStream
//   - Google PubSub
//   - MQTT
//
// See https://cloudevents.github.io/sdk-go/protocol_implementations.html
//
// [protocol.Receiver]: https://pkg.go.dev/github.com/cloudevents/sdk-go/v2/protocol#Receiver
// [protocol.Sender]: https://pkg.go.dev/github.com/cloudevents/sdk-go/v2/protocol#Sender
package cloudevents
