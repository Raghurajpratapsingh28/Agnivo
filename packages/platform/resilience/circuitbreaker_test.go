package resilience_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/resilience"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		Name:         "test",
		MaxFailures:  3,
		OpenDuration: time.Minute,
	})
	errTest := errors.New("fail")

	for i := 0; i < 3; i++ {
		require.Error(t, cb.Call(func() error { return errTest }))
	}
	assert.Equal(t, resilience.StateOpen, cb.State())

	err := cb.Call(func() error { return nil })
	require.Error(t, err)
}

func TestCircuitBreaker_RecoversAfterSuccess(t *testing.T) {
	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{Name: "test", MaxFailures: 2})
	require.Error(t, cb.Call(func() error { return errors.New("x") }))
	require.NoError(t, cb.Call(func() error { return nil }))
	assert.Equal(t, resilience.StateClosed, cb.State())
}
