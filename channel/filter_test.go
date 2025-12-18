package channel

import (
	"testing"

	"github.com/fxsml/gopipe/channel/internal/test"
)

func TestFilter(t *testing.T) {
	test.RunFilter_Even(t, Filter)
	test.RunFilter_AllFalse(t, Filter)
	test.RunFilter_Closure(t, Filter)
}
