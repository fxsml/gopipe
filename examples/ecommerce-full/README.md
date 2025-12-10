# E-Commerce Full Example

Demonstrates a complete e-commerce system using gopipe pubsub with multiple broker types.

## Architecture

```
[HTTP POST :8080/commands/*] → HTTPReceiver
                                  ↓
                           MultiplexReceiver
                                  ↓
                            Subscriber (polls)
                                  ↓
                               Router
                                  ↓
               ┌──────────────────┴──────────────────┐
               ↓                                     ↓
      CommandHandlers                         EventHandlers
      (CreateOrder)                          (ProcessPayment)
               ↓                                     ↓
         OrderCreated                        PaymentProcessed
               ↓                                     ↓
       MultiplexSender ────────────────────> MultiplexSender
               ↓                                     ↓
    ┌──────────┴──────────┐              ┌──────────┴──────────┐
    ↓                     ↓              ↓                     ↓
ChannelBroker        HTTPSender     ChannelBroker        HTTPSender
(internal/*)         (events/*)     (internal/*)         (events/*)
    ↓                     ↓              ↓                     ↓
 cascading          [Webhook :8081]   cascading          [Webhook :8081]
```

## Components

### Brokers
- **HTTPReceiver** (port 8080): Accepts incoming commands via HTTP POST
- **ChannelBroker**: In-process messaging for internal cascading
- **HTTPSender** (port 8081): Sends events as webhooks to external systems

### Handlers
- `CreateOrderHandler`: Creates order, emits `OrderCreated` event
- `OrderCreatedHandler`: Reacts to event, emits `ReserveInventory` command (cascading)
- `ReserveInventoryHandler`: Reserves stock, emits `InventoryReserved` event
- `InventoryReservedHandler`: Confirms order, emits `OrderConfirmed` event

### Flow
1. HTTP POST `/commands/CreateOrder` with order JSON
2. Router dispatches to `CreateOrderHandler`
3. Handler returns `OrderCreated` event → published to `events/order/created`
4. `OrderCreatedHandler` catches event → emits `ReserveInventory` to `internal/commands`
5. Cascades until `OrderConfirmed` event sent to webhook

## Running

```bash
go run ./examples/ecommerce-full/

# In another terminal, send a command:
curl -X POST http://localhost:8080/commands/CreateOrder \
  -H "Content-Type: application/json" \
  -H "X-Gopipe-Prop-Subject: CreateOrder" \
  -d '{"order_id":"ORD-001","customer_id":"CUST-123","items":[{"product_id":"PROD-001","quantity":2,"price":29.99}]}'
```

## Issues Found

See [ISSUES.md](./ISSUES.md) for detailed analysis of problems discovered during implementation.

## Reference

This example was inspired by [Watermill](https://github.com/ThreeDotsLabs/watermill) patterns:
- Channel pub/sub with explicit ack/nack
- HTTP subscriber with response code handling based on acknowledgment
