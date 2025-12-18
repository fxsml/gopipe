package channel

import (
	"testing"

	"github.com/fxsml/gopipe/channel/internal/test"
)

func TestTransform(t *testing.T) {
	test.RunTransform_Success(t, Transform)
}
