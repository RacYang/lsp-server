package metrics

import (
	"errors"
	"testing"
	"time"
)

func TestObserveStorage(t *testing.T) {
	started := time.Now().Add(-time.Millisecond)
	ObserveStorage("test", "ok", started, nil)
	ObserveStorage("test", "err", started, errors.New("boom"))
}
