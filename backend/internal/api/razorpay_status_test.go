package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"revenue-recovery/backend/internal/integrations/razorpay"
)

func TestRazorpayStatusIsAuthenticatedAndSecretSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"items":[]}`))
	}))
	defer provider.Close()
	client := razorpay.NewClient(provider.URL, "rzp_test_fixture", "secret-fixture")
	router := gin.New()
	NewRazorpayStatus(client, "local", true, false).Register(router.Group("/api/v1"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/integrations/razorpay/status", nil))
	if response.Code != 200 {
		t.Fatalf("status %d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["authenticated"] != true || body["mode"] != "test" || body["selected_provider"] != "local" {
		t.Fatalf("body=%v", body)
	}
	rendered := strings.ToLower(response.Body.String())
	for _, secret := range []string{"secret-fixture", "rzp_test_fixture", "authorization"} {
		if strings.Contains(rendered, strings.ToLower(secret)) {
			t.Fatalf("status leaked %q", secret)
		}
	}
}
