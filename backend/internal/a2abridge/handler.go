package a2abridge

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxJSONRPCBody int64 = 16 << 10

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

// AgentCard is intentionally discoverable only when the local bridge is fully
// configured. It advertises planning semantics and authentication, not HAI's
// internal tools, users, sources, or configuration.
func (h *Handler) AgentCard(c *gin.Context) {
	card, err := h.service.AgentCard()
	if errors.Is(err, ErrUnavailable) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.JSON(http.StatusOK, card)
}

func (h *Handler) Send(c *gin.Context) {
	if !h.service.Authorize(bearerToken(c.GetHeader("Authorization"))) {
		// A disabled bridge and an invalid token intentionally have the same
		// response, so the endpoint cannot become an environment oracle.
		c.Status(http.StatusNotFound)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONRPCBody)
	var request jsonRPCRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		writeRPCError(c, nil, -32600, "invalid JSON-RPC request")
		return
	}
	if request.JSONRPC != "2.0" || len(request.ID) == 0 || string(request.ID) == "null" {
		writeRPCError(c, request.ID, -32600, "JSON-RPC 2.0 request id is required")
		return
	}
	if request.Method != "tasks/send" {
		writeRPCError(c, request.ID, -32601, "only tasks/send is supported by this local planning bridge")
		return
	}
	text, err := taskText(request.Params)
	if err != nil {
		writeRPCError(c, request.ID, -32602, "tasks/send requires one bounded user text message")
		return
	}
	proposal, err := h.service.Draft(text)
	if errors.Is(err, ErrInvalidInput) {
		writeRPCError(c, request.ID, -32602, "task text must be non-empty and at most 4096 characters")
		return
	}
	if err != nil {
		writeRPCError(c, request.ID, -32603, "HAI could not create a controlled planning draft")
		return
	}
	c.JSON(http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: request.ID, Result: a2aTask{
		ID: uuid.NewString(), Status: taskStatus{State: "completed", Message: a2aMessage{
			Role: "agent", Parts: []a2aPart{{Kind: "text", Text: "HAI returned a non-executable planning draft. Review it in HAI before creating, approving, or running any work."}},
		}},
		Artifacts: []a2aArtifact{{Name: "hai-controlled-planning-proposal", Parts: []a2aPart{{Kind: "data", Data: proposal}}}},
	}})
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type sendParams struct {
	Message a2aMessage `json:"message"`
}

type a2aMessage struct {
	Role  string    `json:"role"`
	Parts []a2aPart `json:"parts"`
}

type a2aPart struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	Data any    `json:"data,omitempty"`
}

type a2aTask struct {
	ID        string        `json:"id"`
	Status    taskStatus    `json:"status"`
	Artifacts []a2aArtifact `json:"artifacts"`
}

type taskStatus struct {
	State   string     `json:"state"`
	Message a2aMessage `json:"message"`
}

type a2aArtifact struct {
	Name  string    `json:"name"`
	Parts []a2aPart `json:"parts"`
}

func taskText(raw json.RawMessage) (string, error) {
	var params sendParams
	if len(raw) == 0 || json.Unmarshal(raw, &params) != nil || strings.ToLower(strings.TrimSpace(params.Message.Role)) != "user" || len(params.Message.Parts) == 0 || len(params.Message.Parts) > 4 {
		return "", ErrInvalidInput
	}
	parts := make([]string, 0, len(params.Message.Parts))
	for _, part := range params.Message.Parts {
		if strings.ToLower(strings.TrimSpace(part.Kind)) != "text" || strings.TrimSpace(part.Text) == "" || part.Data != nil {
			return "", ErrInvalidInput
		}
		parts = append(parts, part.Text)
	}
	text := normalize(strings.Join(parts, " "))
	if text == "" || utf8.RuneCountInString(text) > 4096 {
		return "", ErrInvalidInput
	}
	return text, nil
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func writeRPCError(c *gin.Context, id json.RawMessage, code int, message string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	c.JSON(http.StatusBadRequest, jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &jsonRPCError{Code: code, Message: message}})
}
