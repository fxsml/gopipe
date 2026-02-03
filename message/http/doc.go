// Package http provides HTTP pub/sub for CloudEvents using standard library net/http.
//
// This package uses the CloudEvents SDK for protocol handling, supporting:
//   - Binary content mode (metadata in Ce-* headers, efficient, default)
//   - Structured content mode (metadata in JSON body, opt-in)
//   - Batch content mode (JSON array of events)
//
// # Subscriber
//
// [Subscriber] implements [http.Handler] and delivers CloudEvents to a channel.
// Use standard [http.ServeMux] for topic routing:
//
//	orders := http.NewSubscriber(http.SubscriberConfig{BufferSize: 100})
//
//	mux := http.NewServeMux()
//	mux.Handle("/events/orders", orders)
//
//	ctx, cancel := context.WithCancel(context.Background())
//	ch, _ := orders.Subscribe(ctx)
//
//	go http.ListenAndServe(":8080", mux)
//
//	for msg := range ch {
//	    // process
//	    msg.Ack()
//	}
//
// # Publisher
//
// [Publisher] sends CloudEvents over HTTP. Uses binary mode by default.
// Use StructuredMode config for structured mode if required by receiver.
//
//	pub := http.NewPublisher(http.PublisherConfig{
//	    TargetURL: "http://localhost:8080/events",
//	})
//
//	// Single send (binary mode by default)
//	pub.Send(ctx, msg)
//
//	// Channel-based publish (default: sends individually)
//	done, _ := pub.Publish(ctx, inputCh)
//
//	// Batch mode (collects messages and sends as JSON array)
//	pub := http.NewPublisher(http.PublisherConfig{
//	    TargetURL:     "http://localhost:8080/events",
//	    BatchSize:     100,              // Collect up to 100 messages
//	    BatchDuration: time.Second,      // Or flush every second
//	})
//	done, _ := pub.Publish(ctx, inputCh)
//
// Internally, Publish always uses batch mode for consistent middleware support.
// When BatchSize=1 (default), messages are sent individually using Send for efficiency.
package http
