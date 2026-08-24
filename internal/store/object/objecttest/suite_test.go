package objecttest

import (
	"testing"

	"github.com/mikerudolph/artifacts/internal/store/object"
)

func TestMemConformance(t *testing.T) {
	t.Parallel()
	Run(t, func(testing.TB) object.Store {
		return NewMem()
	})
}
