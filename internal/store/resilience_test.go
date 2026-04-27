package store

import (
	"context"
	"errors"
	"testing"
)

func TestRetryStopsAfterRetryableError(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), "test", "retry", 2, func(context.Context) error {
		attempts++
		if attempts == 1 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d", attempts)
	}
}

func TestRetryDoesNotRetryFatalError(t *testing.T) {
	attempts := 0
	want := errors.New("fatal")
	err := Retry(context.Background(), "test", "fatal", 3, func(context.Context) error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d", attempts)
	}
}
