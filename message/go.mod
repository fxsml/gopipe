module github.com/fxsml/gopipe/message

go 1.24.1

require (
	github.com/cloudevents/sdk-go/v2 v2.16.2
	github.com/fxsml/gopipe/channel v0.16.0
	github.com/fxsml/gopipe/pipe v0.16.0
	github.com/google/uuid v1.6.0
	github.com/redis/go-redis/v9 v9.18.0
)

require (
	github.com/alicebob/miniredis/v2 v2.37.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
)

replace github.com/fxsml/gopipe/pipe => ../pipe
