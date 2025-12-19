module github.com/fxsml/gopipe/examples/adapters

go 1.24.1

require (
	github.com/fxsml/gopipe v0.0.0
	github.com/nats-io/nats.go v1.47.0
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/segmentio/kafka-go v0.4.49
)

replace github.com/fxsml/gopipe => ../..
