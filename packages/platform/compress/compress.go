// Package compress provides gzip compression helpers for in-memory byte slices,
// used to shrink large payloads (build logs, cached artifacts) before storing
// them in the database or object storage.
package compress

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Gzip compresses data at the given level (gzip.BestSpeed..gzip.BestCompression,
// or gzip.DefaultCompression). It returns a self-contained gzip stream.
func Gzip(data []byte, level int) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(data) / 2)
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidArgument, "compress: invalid level")
	}
	if _, err := w.Write(data); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "compress: write")
	}
	if err := w.Close(); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "compress: close")
	}
	return buf.Bytes(), nil
}

// Gunzip decompresses a gzip stream produced by Gzip. A maxBytes <= 0 imposes no
// limit; a positive maxBytes guards against decompression bombs by capping the
// number of decompressed bytes.
func Gunzip(data []byte, maxBytes int64) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidArgument, "compress: invalid gzip stream")
	}
	defer func() { _ = r.Close() }()

	var reader io.Reader = r
	if maxBytes > 0 {
		// +1 so we can detect when the stream exceeds the cap.
		reader = io.LimitReader(r, maxBytes+1)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "compress: read")
	}
	if maxBytes > 0 && int64(len(out)) > maxBytes {
		return nil, errors.New(errors.CodeInvalidArgument, "compress: decompressed size exceeds limit")
	}
	return out, nil
}
