package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// Media type constants for content negotiation.
const (
	MediaJSON        = "application/json"
	MediaNDJSON      = "application/x-ndjson"
	MediaText        = "text/plain"
	MediaHTML        = "text/html"
	MediaEventStream = "text/event-stream"
)

// Negotiate picks the best matching media type from offered based on the
// request's Accept header. It returns the matched type and whether a match was
// found. When Accept is */* or empty, the first offered type is returned.
func Negotiate(r *http.Request, offered ...string) (string, bool) {
	if len(offered) == 0 {
		return "", false
	}
	accept := Header(r, "Accept")
	if accept == "" || accept == "*/*" {
		return offered[0], true
	}

	bestQ := -1.0
	best := ""
	for _, part := range strings.Split(accept, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		media := part
		q := 1.0
		if i := strings.Index(part, ";"); i >= 0 {
			media = strings.TrimSpace(part[:i])
			if qStr, ok := strings.CutPrefix(strings.TrimSpace(part[i+1:]), "q="); ok {
				if f, err := strconv.ParseFloat(qStr, 64); err == nil {
					q = f
				}
			}
		}
		if media == "*/*" {
			if q > bestQ {
				bestQ = q
				best = offered[0]
			}
			continue
		}
		for _, o := range offered {
			if mediaMatches(media, o) && q > bestQ {
				bestQ = q
				best = o
			}
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// WantsJSON reports whether the client prefers JSON based on Accept.
func WantsJSON(r *http.Request) bool {
	t, ok := Negotiate(r, MediaJSON, MediaHTML, MediaText)
	return ok && t == MediaJSON
}

func mediaMatches(accept, offered string) bool {
	if accept == offered {
		return true
	}
	if strings.HasSuffix(accept, "/*") {
		prefix := strings.TrimSuffix(accept, "/*")
		return strings.HasPrefix(offered, prefix+"/")
	}
	return false
}
