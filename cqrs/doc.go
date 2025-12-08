// Package cqrs provides Command Query Responsibility Segregation (CQRS) patterns for gopipe.
//
// # Overview
//
// This package implements CQRS and Saga patterns on top of gopipe's message package.
// It provides a clean, type-safe API for building event-driven architectures.
//
// # Architecture
//
//	Commands → Command Handlers → Events → Event Handlers (Side Effects)
//	                                    → Saga Coordinator → Commands (Workflow)
//
// # Core Components
//
//   - NewCommandHandler: Creates handlers that process commands and return events
//   - NewEventHandler: Creates handlers that process events and perform side effects
//   - SagaCoordinator: Interface for coordinating multi-step workflows
//   - Marshaler: Pluggable serialization (JSON, Protobuf, etc.)
//
// # Design Principles
//
//  1. Command handlers are pure functions: Command → Events
//  2. Event handlers perform side effects only (emails, logging, analytics)
//  3. Saga coordinators define workflow logic separately from side effects
//  4. Processors return channels (gopipe pattern)
//  5. Acking is independent per stage, correlation IDs for tracing
//
// # Example: Simple CQRS
//
//	marshaler := NewJSONMarshaler()
//
//	// Command handler
//	createOrderHandler := NewCommandHandler(
//	    "CreateOrder",
//	    marshaler,
//	    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
//	        saveOrder(cmd)
//	        return []OrderCreated{{ID: cmd.ID, Amount: cmd.Amount}}, nil
//	    },
//	)
//
//	// Event handler (side effect)
//	emailHandler := NewEventHandler(
//	    "OrderCreated",
//	    marshaler,
//	    func(ctx context.Context, evt OrderCreated) error {
//	        return emailService.Send(evt.CustomerID, "Order created!")
//	    },
//	)
//
//	// Wire together
//	commandRouter := message.NewRouter(message.RouterConfig{}, createOrderHandler)
//	eventRouter := message.NewRouter(message.RouterConfig{}, emailHandler)
//
//	commands := make(chan *message.Message)
//	events := commandRouter.Start(ctx, commands)
//	eventRouter.Start(ctx, events)
//
// # Example: Saga Coordinator
//
//	type OrderSagaCoordinator struct {
//	    marshaler Marshaler
//	}
//
//	func (s *OrderSagaCoordinator) OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
//	    subject, _ := msg.Properties.Subject()
//
//	    switch subject {
//	    case "OrderCreated":
//	        var evt OrderCreated
//	        s.marshaler.Unmarshal(msg.Payload, &evt)
//
//	        // Workflow: OrderCreated → ChargePayment + ReserveInventory
//	        corrID, _ := msg.Properties.CorrelationID()
//	        return CreateCommands(s.marshaler, corrID,
//	            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
//	            ReserveInventory{OrderID: evt.ID},
//	        ), nil
//
//	    case "PaymentCharged":
//	        // Workflow: PaymentCharged → ShipOrder
//	        var evt PaymentCharged
//	        s.marshaler.Unmarshal(msg.Payload, &evt)
//	        corrID, _ := msg.Properties.CorrelationID()
//	        return CreateCommands(s.marshaler, corrID,
//	            ShipOrder{OrderID: evt.OrderID},
//	        ), nil
//
//	    case "OrderShipped":
//	        return nil, nil // Terminal
//	    }
//
//	    return nil, nil
//	}
//
//	// Wire together with feedback loop
//	initialCommands := make(chan *message.Message, 10)
//	sagaCommands := make(chan *message.Message, 100)
//	allCommands := channel.Merge(initialCommands, sagaCommands)
//
//	events := commandRouter.Start(ctx, allCommands)
//
//	// Fan-out events to side effects AND saga coordinator
//	eventChan1 := make(chan *message.Message, 100)
//	eventChan2 := make(chan *message.Message, 100)
//	go func() {
//	    for evt := range events {
//	        eventChan1 <- evt
//	        eventChan2 <- evt
//	    }
//	    close(eventChan1)
//	    close(eventChan2)
//	}()
//
//	sideEffectsRouter.Start(ctx, eventChan1)
//	sagaOut := sagaRouter.Start(ctx, eventChan2)
//
//	// Feedback loop: saga commands → command processor
//	go func() {
//	    for cmd := range sagaOut {
//	        sagaCommands <- cmd
//	    }
//	}()
//
// # Benefits
//
//   - Type-safe: Generic handlers eliminate casting
//   - Decoupled: Saga coordinators separate workflow from side effects
//   - Testable: Pure functions, easy to test
//   - Composable: Built on gopipe's channel-based architecture
//   - Pluggable: Custom marshalers, saga coordinators, stores
//
// # Advanced Patterns
//
// For advanced patterns like compensating sagas and transactional outbox,
// see the architecture documentation:
//
//   - docs/cqrs-architecture-overview.md
//   - docs/cqrs-advanced-patterns.md
//
// These patterns will be implemented in separate packages:
//
//   - cqrs/compensation - Compensating saga pattern with automatic rollback
//   - cqrs/outbox - Transactional outbox pattern for exactly-once semantics
package cqrs
