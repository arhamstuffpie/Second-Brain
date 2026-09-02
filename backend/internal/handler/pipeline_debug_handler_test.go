package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPipelineDebugRequestedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &pipelineDebugHandler{adminUserID: "admin-owner"}

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/debug/pipeline/overview", nil)
	if owner := handler.requestedOwner(context); owner != "admin-owner" {
		t.Fatalf("default owner = %q", owner)
	}

	selectedContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	selectedContext.Request = httptest.NewRequest("GET", "/debug/pipeline/overview?owner_user_id=selected-owner", nil)
	if owner := handler.requestedOwner(selectedContext); owner != "selected-owner" {
		t.Fatalf("selected owner = %q", owner)
	}
}
