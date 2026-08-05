// Package cloudevents provides integration between gopipe's [message] package
// and the CloudEvents SDK protocol bindings.
//
// This package wraps CloudEvents [protocol.Receiver] and [protocol.Sender] interfaces,
// bridging their acknowledgment model (Finish) to gopipe's callback-based [message.Acking].
//
// # Usage
//
// Use [Subscriber] and [Publisher] directly, composed with [message.Router] via
// [message.NewUnmarshalPipe] and [message.NewMarshalPipe]:
//
//	// Create protocol instances (using NATS as example)
//	natsReceiver, _ := nats_ce.NewConsumer(conn, subject)
//	kafkaSender, _ := kafka_ce.NewSender(brokers, topic)
//
//	sub := cloudevents.NewSubscriber(natsReceiver, cloudevents.SubscriberConfig{})
//	pub := cloudevents.NewPublisher(kafkaSender, cloudevents.PublisherConfig{})
//
//	rawIn, _ := sub.Subscribe(ctx)
//
//	router := message.NewRouter(message.PipeConfig{})
//	router.AddHandler("orders", nil, handler)
//
//	unmarshal := message.NewUnmarshalPipe(router, message.NewJSONMarshaler(), message.PipeConfig{})
//	typedIn, _ := unmarshal.Pipe(ctx, rawIn)
//	typedOut, _ := router.Pipe(ctx, typedIn)
//	marshal := message.NewMarshalPipe(message.NewJSONMarshaler(), message.PipeConfig{})
//	rawOut, _ := marshal.Pipe(ctx, typedOut)
//
//	pub.Publish(ctx, rawOut)
//
// # Resource Cleanup
//
// Both [PublisherConfig] and [SubscriberConfig] accept an optional CleanupHandler
// that is invoked when the publisher or subscriber finishes (input channel closes,
// context cancels, or receiver returns EOF). Use it to close underlying
// protocol-layer resources without manual tracking:
//
//	pub := cloudevents.NewPublisher(sender, cloudevents.PublisherConfig{
//	    CleanupHandler: func(ctx context.Context) {
//	        sender.Close(ctx)
//	    },
//	})
//
// # Acknowledgment Bridge
//
// CloudEvents uses Finish(err) for acknowledgment:
//   - Finish(nil) = ACK (successful processing)
//   - Finish(err) = NACK (failed processing)
//
// This package bridges to gopipe's [message.Acking]:
//   - [message.Message.Ack] calls ceMsg.Finish(nil)
//   - [message.Message.Nack] calls ceMsg.Finish(err)
//
// # Conversion Functions
//
// For manual conversion between CloudEvents and gopipe messages:
//   - [FromCloudEvent] converts a CloudEvents event to a [message.Message] with raw []byte Data
//   - [ToCloudEvent] converts a [message.Message] with raw []byte Data to a CloudEvents event
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
