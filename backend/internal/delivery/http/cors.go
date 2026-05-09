package http

import (
	nethttp "net/http"
	"strings"
)

const (
	corsAllowedMethods = "GET, POST, OPTIONS"
	corsAllowedHeaders = "Content-Type, Authorization"
	corsMaxAge         = "600"
)

func corsMiddleware(allowedOrigins []string) func(nethttp.Handler) nethttp.Handler {
	origins := make(map[string]struct{}, len(allowedOrigins))
	allowAll := false
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			allowAll = true
			continue
		}
		origins[trimmed] = struct{}{}
	}

	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				appendVary(w.Header(), "Origin")
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if _, ok := origins[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			if r.Method == nethttp.MethodOptions {
				if w.Header().Get("Access-Control-Allow-Origin") != "" {
					w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
					w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
					w.Header().Set("Access-Control-Max-Age", corsMaxAge)
				}
				w.WriteHeader(nethttp.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func appendVary(header nethttp.Header, value string) {
	current := header.Get("Vary")
	if current == "" {
		header.Set("Vary", value)
		return
	}

	for _, part := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(part), value) {
			return
		}
	}

	header.Set("Vary", current+", "+value)
}
