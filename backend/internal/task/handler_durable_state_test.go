package task

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestTaskHandlersReturnOnlyDurableOwnerScopedState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := NewMemoryTaskStateRepository()
	base := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	for _, owner := range []string{"alice", "bob"} {
		if err := repository.AppendCompletionPlan(
			owner,
			taskStateTestPlan(owner+"-plan", owner, base),
		); err != nil {
			t.Fatalf("append %s completion plan: %v", owner, err)
		}
		if _, err := repository.CreateReviewItem(
			owner,
			taskStateTestReviewItem(owner, owner+"-plan", base),
		); err != nil {
			t.Fatalf("create %s review item: %v", owner, err)
		}
	}
	taskService := &service{
		stateRepository: repository,
		logs: []CompletionPlan{
			taskStateTestPlan("mirror-bob-plan", "bob", base),
		},
		reviewQueue: []ReviewQueueItem{
			taskStateTestReviewItem("bob", "mirror-bob-plan", base),
		},
	}
	handler := NewHandler(taskService)

	logs := performDurableTaskHandlerRequest(t, handler.Logs, "/task/logs", "alice")
	if logs.Code != http.StatusOK {
		t.Fatalf("logs status = %d, want 200: %s", logs.Code, logs.Body.String())
	}
	if !strings.Contains(logs.Body.String(), `"id":"alice-plan"`) ||
		strings.Contains(logs.Body.String(), `"id":"bob-plan"`) ||
		strings.Contains(logs.Body.String(), `"id":"mirror-bob-plan"`) {
		t.Fatalf("owner-scoped durable logs leaked another owner or mirror state: %s", logs.Body.String())
	}

	queue := performDurableTaskHandlerRequest(t, handler.ReviewQueue, "/task/review-queue", "alice")
	if queue.Code != http.StatusOK {
		t.Fatalf("queue status = %d, want 200: %s", queue.Code, queue.Body.String())
	}
	if !strings.Contains(queue.Body.String(), `"taskId":"alice-plan"`) ||
		strings.Contains(queue.Body.String(), `"taskId":"bob-plan"`) ||
		strings.Contains(queue.Body.String(), `"taskId":"mirror-bob-plan"`) {
		t.Fatalf("owner-scoped durable queue leaked another owner or mirror state: %s", queue.Body.String())
	}
}

func TestTaskHandlersReturn500WhenDurableStateReadsFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repositoryFailure := errors.New("durable task ledger unavailable")
	repository := &failingTaskStateReadRepository{
		TaskStateRepository: NewMemoryTaskStateRepository(),
		logsError:           repositoryFailure,
		queueError:          repositoryFailure,
	}
	taskService := &service{
		stateRepository: repository,
		logs: []CompletionPlan{
			taskStateTestPlan("misleading-mirror-plan", "alice", time.Now().UTC()),
		},
		reviewQueue: []ReviewQueueItem{
			taskStateTestReviewItem("alice", "misleading-mirror-plan", time.Now().UTC()),
		},
	}
	handler := NewHandler(taskService)

	for _, test := range []struct {
		name    string
		path    string
		invoke  func(*gin.Context)
		message string
	}{
		{
			name:    "completion logs",
			path:    "/task/logs",
			invoke:  handler.Logs,
			message: "task history is temporarily unavailable",
		},
		{
			name:    "review queue",
			path:    "/task/review-queue",
			invoke:  handler.ReviewQueue,
			message: "task review queue is temporarily unavailable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performDurableTaskHandlerRequest(t, test.invoke, test.path, "alice")
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500: %s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.message) {
				t.Fatalf("response did not explain durable-state outage: %s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), "misleading-mirror-plan") ||
				strings.TrimSpace(response.Body.String()) == "[]" {
				t.Fatalf("repository failure was misrepresented as available or empty state: %s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), repositoryFailure.Error()) {
				t.Fatalf("internal repository error leaked to client: %s", response.Body.String())
			}
		})
	}
}

type failingTaskStateReadRepository struct {
	TaskStateRepository
	logsError  error
	queueError error
}

func (r *failingTaskStateReadRepository) ListCompletionPlans(
	string,
	int,
) ([]CompletionPlan, error) {
	return nil, r.logsError
}

func (r *failingTaskStateReadRepository) ListReviewItems(
	string,
	int,
) ([]ReviewQueueItem, error) {
	return nil, r.queueError
}

func performDurableTaskHandlerRequest(
	t *testing.T,
	invoke func(*gin.Context),
	path string,
	ownerIdentity string,
) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, path, nil)
	context.Set(identity.ContextSubjectKey, ownerIdentity)
	invoke(context)
	return response
}
