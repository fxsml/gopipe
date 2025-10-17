package gopipe

import (
	"context"
	"testing"
	"time"

	"github.com/fxsml/gopipe/internal/test"
)

func TestBatch(t *testing.T) {
	f := func(in <-chan int, handle func([]int) []int, maxSize int, maxDuration time.Duration) <-chan int {
		handlePipe := func(_ context.Context, batch []int) ([]int, error) {
			return handle(batch), nil
		}
		return NewBatchPipe(
			handlePipe,
			maxSize,
			maxDuration,
		).Start(context.Background(), in)
	}
	test.RunBatch_Success(t, f)
}

func TestFilter(t *testing.T) {
	f := func(in <-chan int, handle func(int) bool) <-chan int {
		handlePipe := func(_ context.Context, v int) (bool, error) {
			return handle(v), nil
		}
		return NewFilterPipe(handlePipe).Start(context.Background(), in)
	}
	test.RunFilter_Even(t, f)
	test.RunFilter_AllFalse(t, f)
	test.RunFilter_Closure(t, f)
}

func TestProcess(t *testing.T) {
	f := func(in <-chan int, handle func(int) []string) <-chan string {
		// Adapter to use NewProcessPipe with the same signature
		handlePipe := func(_ context.Context, val int) ([]string, error) {
			return handle(val), nil
		}
		return NewProcessPipe(
			handlePipe,
		).Start(context.Background(), in)
	}
	test.RunProcess_Success(t, f)
}

func TestSink(t *testing.T) {
	f := func(in <-chan int, handle func(int)) <-chan struct{} {
		handlePipe := func(_ context.Context, in int) error {
			handle(in)
			return nil
		}
		return NewSinkPipe(handlePipe).Start(context.Background(), in)
	}
	test.RunSink_handleCalled(t, f)
	test.RunSink_ExitsOnClose(t, f)
	test.RunSink_EmptyChannel(t, f)
}
