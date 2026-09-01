package correlation

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestMiddlewarePreservesCorrelationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/", func(c *gin.Context) {
		if From(c.Request.Context()) != "trace-123" {
			t.Fatal("correlation ID missing from context")
		}
		c.Status(204)
	})
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("X-Correlation-ID", "trace-123")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Header().Get("X-Correlation-ID") != "trace-123" {
		t.Fatal("correlation ID not returned")
	}
}
