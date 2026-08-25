package hostruntime

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerRequiresDedicatedBridgeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(NewService(newMemoryRepository()), Config{Enabled: true, Token: strings.Repeat("a", 32)})
	router := gin.New()
	handler.RegisterRoutes(router)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/leases", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestHandlerLeasesAndCompletesBoundedJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(newMemoryRepository())
	if _, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: "task-1",
		Prompt: "Inspect the approved workspace.", WorkspaceKey: "hai", Approved: true,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	token := strings.Repeat("a", 32)
	handler := NewHandler(service, Config{Enabled: true, Token: token})
	router := gin.New()
	handler.RegisterRoutes(router)

	leaseRequest := httptest.NewRequest(http.MethodPost, "/leases", nil)
	leaseRequest.Header.Set("Authorization", "Bearer "+token)
	leaseResponse := httptest.NewRecorder()
	router.ServeHTTP(leaseResponse, leaseRequest)
	if leaseResponse.Code != http.StatusOK {
		t.Fatalf("lease status = %d: %s", leaseResponse.Code, leaseResponse.Body.String())
	}
	var lease Lease
	if err := json.Unmarshal(leaseResponse.Body.Bytes(), &lease); err != nil {
		t.Fatalf("decode lease: %v", err)
	}
	confirmPayload := `{"leaseToken":"` + lease.Token + `"}`
	confirmRequest := httptest.NewRequest(http.MethodPost, "/leases/"+lease.Job.ID.String()+"/confirm", bytes.NewBufferString(confirmPayload))
	confirmRequest.Header.Set("Authorization", "Bearer "+token)
	confirmRequest.Header.Set("Content-Type", "application/json")
	confirmResponse := httptest.NewRecorder()
	router.ServeHTTP(confirmResponse, confirmRequest)
	if confirmResponse.Code != http.StatusNoContent {
		t.Fatalf("confirm status = %d: %s", confirmResponse.Code, confirmResponse.Body.String())
	}

	payload := `{"leaseToken":"` + lease.Token + `","exitCode":0,"output":"completed"}`
	completeRequest := httptest.NewRequest(http.MethodPost, "/leases/"+lease.Job.ID.String()+"/complete", bytes.NewBufferString(payload))
	completeRequest.Header.Set("Authorization", "Bearer "+token)
	completeRequest.Header.Set("Content-Type", "application/json")
	completeResponse := httptest.NewRecorder()
	router.ServeHTTP(completeResponse, completeRequest)
	if completeResponse.Code != http.StatusOK {
		t.Fatalf("complete status = %d: %s", completeResponse.Code, completeResponse.Body.String())
	}

	replayResponse := httptest.NewRecorder()
	replayRequest := httptest.NewRequest(http.MethodPost, "/leases/"+lease.Job.ID.String()+"/complete", bytes.NewBufferString(payload))
	replayRequest.Header.Set("Authorization", "Bearer "+token)
	replayRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusConflict {
		t.Fatalf("replay completion status = %d, want %d", replayResponse.Code, http.StatusConflict)
	}
}

func TestHandlerBlocksFinalConfirmationWhenEmergencyStopStartsAfterLease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(newMemoryRepository())
	if _, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: "task-confirm-stopped",
		Prompt: "Inspect the approved workspace.", WorkspaceKey: "hai", Approved: true,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	token := strings.Repeat("a", 32)
	handler := NewHandler(service, Config{Enabled: true, Token: token})
	router := gin.New()
	handler.RegisterRoutes(router)

	leaseRequest := httptest.NewRequest(http.MethodPost, "/leases", nil)
	leaseRequest.Header.Set("Authorization", "Bearer "+token)
	leaseResponse := httptest.NewRecorder()
	router.ServeHTTP(leaseResponse, leaseRequest)
	var lease Lease
	if leaseResponse.Code != http.StatusOK || json.Unmarshal(leaseResponse.Body.Bytes(), &lease) != nil {
		t.Fatalf("lease response = %d: %s", leaseResponse.Code, leaseResponse.Body.String())
	}

	t.Setenv("HAI_EMERGENCY_STOP", "true")
	confirmRequest := httptest.NewRequest(http.MethodPost, "/leases/"+lease.Job.ID.String()+"/confirm", bytes.NewBufferString(`{"leaseToken":"`+lease.Token+`"}`))
	confirmRequest.Header.Set("Authorization", "Bearer "+token)
	confirmRequest.Header.Set("Content-Type", "application/json")
	confirmResponse := httptest.NewRecorder()
	router.ServeHTTP(confirmResponse, confirmRequest)
	if confirmResponse.Code != http.StatusLocked {
		t.Fatalf("confirmation while stopped = %d: %s", confirmResponse.Code, confirmResponse.Body.String())
	}
}

func TestHandlerKeepsWorkerIdleWhenEmergencyStopIsActive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(newMemoryRepository())
	if _, err := service.Enqueue(ApprovedTask{
		OwnerIdentity: "robert@example.test", RuntimeID: "deepseek-harness", TaskID: "task-stopped",
		Prompt: "Inspect the approved workspace.", WorkspaceKey: "hai", Approved: true,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	token := strings.Repeat("a", 32)
	handler := NewHandler(service, Config{Enabled: true, Token: token})
	router := gin.New()
	handler.RegisterRoutes(router)
	t.Setenv("HAI_EMERGENCY_STOP", "true")

	request := httptest.NewRequest(http.MethodPost, "/leases", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("lease while stopped status = %d, want %d", response.Code, http.StatusNoContent)
	}
}
