// Package pubsub provides publish-subscribe messaging abstractions and implementations.
//
// The package defines core interfaces (Sender, Receiver) and provides
// implementations: in-process channel-based, IO streams, and HTTP.
//
// Topic naming uses "/" as separator (e.g., "orders/created").
package pubsub
