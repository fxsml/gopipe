package channel

import (
	"testing"

	"github.com/fxsml/gopipe/internal/test"
)

func TestSink(t *testing.T) {
	test.RunSink_handleCalled(t, Sink)
	test.RunSink_ExitsOnClose(t, Sink)
	test.RunSink_EmptyChannel(t, Sink)
}
