package memory

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func DefaultHandler() *Handler {
	return NewHandler(DefaultService())
}

func (h *Handler) Create(c *gin.Context) {
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	memory, err := h.service.Create(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, memory)
}

func (h *Handler) List(c *gin.Context) {
	includeArchived, _ := strconv.ParseBool(c.Query("includeArchived"))
	memories, err := h.service.FindAll(c.Query("projectKey"), includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, memories)
}

// Query lists memories with search, filtering, sorting, and pagination.
// It preserves the existing List endpoint unchanged and adds a richer,
// paginated envelope for clients that need to browse large memory sets.
func (h *Handler) Query(c *gin.Context) {
	includeArchived, _ := strconv.ParseBool(c.Query("includeArchived"))
	memories, err := h.service.FindAll(c.Query("projectKey"), includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	result := Query(memories, QueryParams{
		Search:   c.Query("q"),
		Kind:     c.Query("kind"),
		Tag:      c.Query("tag"),
		Sort:     c.Query("sort"),
		Order:    c.Query("order"),
		Page:     page,
		PageSize: pageSize,
	})
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	memory, err := h.service.FindByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, memory)
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var request UpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	memory, err := h.service.Update(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, memory)
}

func (h *Handler) Archive(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	memory, err := h.service.Archive(id, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, memory)
}

func (h *Handler) Restore(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	memory, err := h.service.Archive(id, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, memory)
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Retrieve(c *gin.Context) {
	var request RetrieveRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Retrieve(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Export(c *gin.Context) {
	memories, err := h.service.FindAll(c.Query("projectKey"), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"format":   "018-hai-context-memory-v1",
		"memories": memories,
	})
}

func parseID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return uuid.UUID{}, false
	}
	return id, true
}
