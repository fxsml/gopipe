package gopipe

import (
	"context"
	"sync"
)

type FanOut[T any] struct {
	mu      sync.Mutex
	outputs map[<-chan struct{}]chan T
	config  FanOutConfig
}

type FanOutConfig struct{}

func NewFanOut[T any](config FanOutConfig) *FanOut[T] {
	return &FanOut[T]{
		config: config,
	}
}

func (f *FanOut[T]) Add(ctx context.Context) <-chan T {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make(chan T)
	f.outputs[ctx.Done()] = out

	return out
}

func (f *FanOut[T]) Start(ctx context.Context, in <-chan T) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer func() {
			f.mu.Lock()
			for outDone, out := range f.outputs {
				close(out)
				delete(f.outputs, outDone)
			}
			f.mu.Unlock()
			close(done)
		}()

		for {
			if ctx.Err() != nil {
				return
			}

			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				f.mu.Lock()
				for outDone, out := range f.outputs {
					select {
					case <-outDone:
						delete(f.outputs, outDone)
						close(out)
					case <-ctx.Done():
					case out <- v:
					}
				}
				f.mu.Unlock()
			}
		}
	}()

	return done
}
