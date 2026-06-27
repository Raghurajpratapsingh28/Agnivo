package timex_test

import (
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/platform/timex"
	"github.com/stretchr/testify/assert"
)

func TestFakeClock(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := timex.NewFakeClock(start)
	assert.Equal(t, start, c.Now())
	c.Advance(2 * time.Hour)
	assert.Equal(t, start.Add(2*time.Hour), c.Now())
	assert.Equal(t, 2*time.Hour, c.Since(start))
}

func TestMinMaxClamp(t *testing.T) {
	a := time.Unix(100, 0)
	b := time.Unix(200, 0)
	assert.Equal(t, b, timex.MaxTime(a, b))
	assert.Equal(t, a, timex.MinTime(a, b))
	assert.Equal(t, a, timex.Clamp(time.Unix(50, 0), a, b))
	assert.Equal(t, b, timex.Clamp(time.Unix(500, 0), a, b))
	assert.Equal(t, time.Unix(150, 0), timex.Clamp(time.Unix(150, 0), a, b))
}

func TestNowUTC(t *testing.T) {
	assert.Equal(t, time.UTC, timex.NowUTC().Location())
}
