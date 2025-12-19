package channel

import (
	"testing"

	"github.com/fxsml/gopipe/channel/internal/test"
)

func TestBatch(t *testing.T) {
	test.RunBatch_Success(t, Batch)
}
