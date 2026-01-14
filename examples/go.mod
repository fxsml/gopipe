module github.com/fxsml/gopipe/examples

go 1.24.1

require (
	github.com/cloudevents/sdk-go/protocol/nats_jetstream/v2 v2.15.2
	github.com/cloudevents/sdk-go/v2 v2.15.2
	github.com/fxsml/gopipe/channel v0.11.0
	github.com/fxsml/gopipe/message v0.11.0
	github.com/fxsml/gopipe/pipe v0.11.0
	github.com/google/uuid v1.6.0
	github.com/nats-io/nats.go v1.38.0
)

require (
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/nats-io/nkeys v0.4.9 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	go.uber.org/atomic v1.4.0 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.10.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

replace (
	github.com/fxsml/gopipe/channel => ../channel
	github.com/fxsml/gopipe/message => ../message
	github.com/fxsml/gopipe/pipe => ../pipe
)
