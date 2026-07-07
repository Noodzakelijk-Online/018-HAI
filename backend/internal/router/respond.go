package router

import (
	"automation-hub-backend/internal/apierror"

	"github.com/gin-gonic/gin"
)

// respondError writes a structured API error using the shared catalog: the HTTP
// status is derived from the error code and the body is the standard
// {"error": {...}} envelope. Handlers should use this so every error response
// shares one contract.
func respondError(c *gin.Context, err *apierror.Error) {
	c.JSON(err.HTTPStatus(), err.Envelope())
}

// respondErr is a convenience wrapper for building and writing an error in one
// call.
func respondErr(c *gin.Context, code apierror.Code, message string) {
	respondError(c, apierror.New(code, message))
}
