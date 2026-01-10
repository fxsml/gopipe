package channel

import (
	"testing"

	"github.com/fxsml/gopipe/channel/internal/test"
)

func TestSink(t *testing.T) {
	test.RunSink_handleCalled(t, Sink)
	test.RunSink_ExitsOnClose(t, Sink)
	test.RunSink_EmptyChannel(t, Sink)
}
