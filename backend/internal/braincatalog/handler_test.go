package braincatalog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListHandlerPublishesReadOnlyCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler()
	router.GET("/", handler.List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Catalog discovery is read-only") || !strings.Contains(response.Body.String(), "openhands") {
		t.Fatalf("catalog response lacks policy or entry: %s", response.Body.String())
	}
}

func TestGetHandlerReturnsNotFoundForUnknownEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler()
	router.GET("/:id", handler.Get)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
