package httpx

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
)

// StreamJSON writes a JSON array as a chunked HTTP response, invoking fn for
// each element. The response is flushed incrementally so clients can consume
// items as they arrive. fn errors abort the stream and are returned to the
// caller; the response may be partially written.
func StreamJSON(w http.ResponseWriter, fn func(yield func(any) error) error) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("httpx: streaming requires http.Flusher")
	}

	if _, err := w.Write([]byte("[")); err != nil {
		return err
	}
	first := true
	err := fn(func(v any) error {
		if !first {
			if _, err := w.Write([]byte(",")); err != nil {
				return err
			}
		}
		first = false
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("]"))
	return err
}

// StreamNDJSON writes newline-delimited JSON records. Each yielded value is
// marshaled as one JSON object followed by a newline and flushed immediately.
func StreamNDJSON(w http.ResponseWriter, fn func(yield func(any) error) error) error {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("httpx: streaming requires http.Flusher")
	}

	return fn(func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})
}

// SSEWriter wraps a ResponseWriter for Server-Sent Events. Call Event to emit
// events; Close terminates the stream with a final flush.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewSSE prepares w for Server-Sent Events and returns an SSEWriter.
func NewSSE(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("httpx: SSE requires http.Flusher")
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	return &SSEWriter{w: w, flusher: flusher}, nil
}

// Event writes a single SSE event. data is written as one or more "data:"
// lines; id and event type are optional.
func (s *SSEWriter) Event(id, eventType string, data []byte) error {
	bw := bufio.NewWriter(s.w)
	if id != "" {
		if _, err := fmt.Fprintf(bw, "id: %s\n", id); err != nil {
			return err
		}
	}
	if eventType != "" {
		if _, err := fmt.Fprintf(bw, "event: %s\n", eventType); err != nil {
			return err
		}
	}
	for _, line := range splitLines(data) {
		if _, err := fmt.Fprintf(bw, "data: %s\n", line); err != nil {
			return err
		}
	}
	if _, err := bw.WriteString("\n"); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// Ping sends an SSE comment line, useful as a keep-alive.
func (s *SSEWriter) Ping() error {
	if _, err := s.w.Write([]byte(": ping\n\n")); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return []string{""}
	}
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	lines = append(lines, string(data[start:]))
	return lines
}
