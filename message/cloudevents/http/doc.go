// Package http provides HTTP-based pub/sub for CloudEvents using standard library net/http.
//
// # Subscriber
//
// [Subscriber] implements [http.Handler] and delivers CloudEvents to a channel.
// Use standard [http.ServeMux] for topic routing:
//
//	orders := http.NewSubscriber(http.SubscriberConfig{BufferSize: 100})
//	payments := http.NewSubscriber(http.SubscriberConfig{BufferSize: 100})
//
//	mux := http.NewServeMux()
//	mux.Handle("/events/orders", orders)
//	mux.Handle("/events/payments", payments)
//	http.ListenAndServe(":8080", mux)
//
//	// Consume from channels
//	for msg := range orders.C() {
//	    // process
//	    msg.Ack()
//	}
//
// Each HTTP request runs in its own goroutine (no concurrency limit).
// Acking bridges to HTTP response: Ack() = 200 OK, Nack() = 500 Error.
//
// # Publisher
//
// [Publisher] sends CloudEvents over HTTP with support for single, streaming, and batch modes:
//
//	pub := http.NewPublisher(http.PublisherConfig{
//	    TargetURL: "http://localhost:8080/events",
//	})
//
//	// Single publish
//	pub.Publish(ctx, "orders", msg)
//
//	// Streaming (concurrent sends)
//	done, _ := pub.PublishStream(ctx, "orders", inputCh)
//
//	// Batch (collects and sends as JSON array)
//	done, _ := pub.PublishBatch(ctx, "orders", inputCh, http.BatchConfig{
//	    MaxSize:     100,
//	    MaxDuration: time.Second,
//	})
//
// # Content Types
//
// Single events use application/cloudevents+json (structured mode).
// Batch events use application/cloudevents-batch+json per CloudEvents spec.
package http
