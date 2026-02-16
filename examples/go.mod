module github.com/fxsml/gopipe/examples

go 1.24.1

require (
	github.com/fxsml/gopipe/channel v0.16.0
	github.com/fxsml/gopipe/message v0.11.0
	github.com/fxsml/gopipe/pipe v0.16.0
	github.com/google/jsonschema-go v0.4.2
	github.com/google/uuid v1.6.0
)

require (
	github.com/cloudevents/sdk-go/v2 v2.16.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
)

replace (
	github.com/fxsml/gopipe/channel => ../channel
	github.com/fxsml/gopipe/message => ../message
	github.com/fxsml/gopipe/pipe => ../pipe
)
