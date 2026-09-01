package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arham/ai-second-brain/internal/service"
	"github.com/gin-gonic/gin"
)

func TestServiceErrorReturnsReadableMemographFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	ServiceError(context, &service.UnavailableError{
		Dependency: "memograph",
		Cause:      errors.New(`Memograph returned 401: {"detail":"secret upstream response"}`),
	})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	var envelope Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != "MEMOGRAPH_UNAVAILABLE" {
		t.Fatalf("code = %q, want MEMOGRAPH_UNAVAILABLE", envelope.Code)
	}
	if envelope.Message != "Memograph rejected the configured API key or JWT" {
		t.Fatalf("message = %q", envelope.Message)
	}
	if strings.Contains(recorder.Body.String(), "secret upstream response") {
		t.Fatalf("response leaked upstream body: %s", recorder.Body.String())
	}
}
