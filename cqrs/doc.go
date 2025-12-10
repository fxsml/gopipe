// Package cqrs provides Command Query Responsibility Segregation patterns for gopipe.
//
// Core components:
//   - NewCommandHandler: Creates handlers that process commands and return events
//   - NewEventHandler: Creates handlers that process events for side effects
//   - SagaCoordinator: Interface for multi-step workflow coordination
//   - CommandMarshaler/EventMarshaler: Pluggable serialization
package cqrs
