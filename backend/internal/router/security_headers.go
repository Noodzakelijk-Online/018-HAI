package router

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// securityHeadersMiddleware sets conservative, framework-agnostic security
// headers on every response. The backend serves JSON APIs, so a strict
// Content-Security-Policy is safe — except for the Swagger UI, which loads its
// own same-origin assets and is therefore exempted from CSP only.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// Modern guidance: disable the legacy XSS auditor rather than enable it.
		h.Set("X-XSS-Protection", "0")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")

		if !strings.Contains(c.Request.URL.Path, "/swagger") {
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		}

		c.Next()
	}
}
