package verification

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAnswerHandlerIgnoresClientHumanApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingVerificationService{}
	handler := NewHandler(service)
	pursuitID := uuid.NewString()
	body, _ := json.Marshal(AnswerRequest{
		Question:      "May this high-risk action proceed?",
		Mode:          ModeAction,
		PursuitID:     pursuitID,
		HumanApproved: true,
	})
	request := httptest.NewRequest(http.MethodPost, "/verification/answer", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	handler.Answer(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if service.request.HumanApproved {
		t.Fatalf("client approval reached verification service")
	}
	if service.request.PursuitID != pursuitID {
		t.Fatalf("pursuit id = %q, want %q", service.request.PursuitID, pursuitID)
	}
}

type capturingVerificationService struct {
	request AnswerRequest
}

func (s *capturingVerificationService) Answer(request AnswerRequest) (*VerificationResult, error) {
	s.request = request
	return &VerificationResult{}, nil
}

func (s *capturingVerificationService) Runs() ([]models.VerificationRun, error) {
	return nil, nil
}

func (s *capturingVerificationService) RunDetails(id uuid.UUID) (*VerificationResult, error) {
	return nil, nil
}
