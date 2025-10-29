package channel

import (
	"testing"

	"github.com/fxsml/gopipe/internal/test"
)

func TestBatch(t *testing.T) {
	test.RunBatch_Success(t, Batch)
}
