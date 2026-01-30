// Package http provides HTTP-based pub/sub for CloudEvents using standard library net/http.
//
// # Subscriber
//
// [Subscriber] receives CloudEvents over HTTP with topic-based routing:
//
//	sub := http.NewSubscriber(http.SubscriberConfig{
//	    Addr: ":8080",
//	    Path: "/events",
//	})
//
//	orders, _ := sub.Subscribe(ctx, "orders")  // receives on /events/orders
//	go sub.Start(ctx)
//
//	for msg := range orders {
//	    // process message
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
