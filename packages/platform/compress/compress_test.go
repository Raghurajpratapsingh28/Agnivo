package compress_test

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/compress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGzipRoundTrip(t *testing.T) {
	data := bytes.Repeat([]byte("agnivo platform "), 1000)
	c, err := compress.Gzip(data, gzip.BestCompression)
	require.NoError(t, err)
	assert.Less(t, len(c), len(data))

	back, err := compress.Gunzip(c, 0)
	require.NoError(t, err)
	assert.Equal(t, data, back)
}

func TestGunzipRespectsLimit(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 10000)
	c, _ := compress.Gzip(data, gzip.DefaultCompression)
	_, err := compress.Gunzip(c, 100)
	require.Error(t, err)
}

func TestGunzipInvalidStream(t *testing.T) {
	_, err := compress.Gunzip([]byte("not gzip"), 0)
	require.Error(t, err)
}
