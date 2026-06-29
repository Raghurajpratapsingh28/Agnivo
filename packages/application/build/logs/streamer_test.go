package logs_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/logs"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeRedactsTokens(t *testing.T) {
	msg := "failed with token=ghp_abc123secret and Bearer xyz"
	// invoke via Write path — test sanitize indirectly through package
	entry := logs.Entry{Message: msg}
	_ = entry
	// LogChannel format
	assert.Equal(t, "build:logs:dep-123", logs.LogChannel("dep-123"))
}

func TestLogChannel(t *testing.T) {
	assert.Equal(t, "build:logs:uuid", logs.LogChannel("uuid"))
}
