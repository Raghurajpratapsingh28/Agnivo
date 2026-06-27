package middleware

import (
	"net/http"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Timeout bounds request processing, returning 503 when exceeded. A zero or
// negative duration disables the timeout.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return middleware.Timeout(d)
}

// Compression applies gzip compression to eligible responses.
func Compression() func(http.Handler) http.Handler {
	return middleware.Compress(5)
}

// CORS builds a CORS middleware from configuration.
func CORS(c config.CORS) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   c.AllowedOrigins,
		AllowedMethods:   c.AllowedMethods,
		AllowedHeaders:   c.AllowedHeaders,
		AllowCredentials: c.AllowCredentials,
		MaxAge:           c.MaxAge,
	})
}

// SecurityHeaders sets conservative, broadly-safe security response headers.
// When isProduction is true, additional headers (HSTS when TLS, CSP) are applied.
func SecurityHeaders(isProduction bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("X-XSS-Protection", "0")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			if isProduction {
				h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
				if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
					h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
