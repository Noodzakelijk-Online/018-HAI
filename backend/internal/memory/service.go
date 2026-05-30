package memory

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CreateRequest struct {
	ProjectKey  string   `json:"projectKey,omitempty"`
	Kind        string   `json:"kind"`
	Content     string   `json:"content"`
	Summary     string   `json:"summary,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
	SourceURI   string   `json:"sourceUri,omitempty"`
	SourceLabel string   `json:"sourceLabel,omitempty"`
}

type UpdateRequest struct {
	ProjectKey  string   `json:"projectKey,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Content     string   `json:"content,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
	SourceURI   string   `json:"sourceUri,omitempty"`
	SourceLabel string   `json:"sourceLabel,omitempty"`
	Archived    *bool    `json:"archived,omitempty"`
}

type RetrieveRequest struct {
	Query      string `json:"query"`
	ProjectKey string `json:"projectKey,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type RankedMemory struct {
	Memory      models.ContextMemory `json:"memory"`
	Score       float64              `json:"score"`
	Explanation string               `json:"explanation"`
}

type RetrieveResult struct {
	Query       string         `json:"query"`
	ProjectKey  string         `json:"projectKey,omitempty"`
	UsedContext []RankedMemory `json:"usedContext"`
	Explanation string         `json:"explanation"`
}

type Service interface {
	Create(request CreateRequest) (*models.ContextMemory, error)
	Update(id uuid.UUID, request UpdateRequest) (*models.ContextMemory, error)
	FindAll(projectKey string, includeArchived bool) ([]models.ContextMemory, error)
	FindByID(id uuid.UUID) (*models.ContextMemory, error)
	Archive(id uuid.UUID, archived bool) (*models.ContextMemory, error)
	Delete(id uuid.UUID) error
	Retrieve(request RetrieveRequest) (*RetrieveResult, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func DefaultService() Service {
	return NewService(DefaultRepository())
}

func (s *service) Create(request CreateRequest) (*models.ContextMemory, error) {
	content := strings.TrimSpace(request.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	kind := firstNonEmpty(strings.TrimSpace(request.Kind), "project")
	projectKey := strings.TrimSpace(request.ProjectKey)
	contentHash := hashContent(projectKey, kind, content)

	if existing, err := s.repo.FindByHash(projectKey, kind, contentHash); err == nil {
		return s.mergeExact(existing, request)
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}

	memories, err := s.repo.FindAll(projectKey, false)
	if err != nil {
		return nil, err
	}
	for _, candidate := range memories {
		if candidate.Kind == kind && similarity(candidate.Content, content) >= 0.78 {
			copyCandidate := candidate
			return s.mergeSimilar(&copyCandidate, request)
		}
	}

	memory := &models.ContextMemory{
		ProjectKey:  projectKey,
		Kind:        kind,
		Content:     content,
		Summary:     compactSummary(firstNonEmpty(request.Summary, content)),
		Tags:        joinTags(request.Tags),
		Confidence:  normalizeConfidence(request.Confidence),
		SourceURI:   strings.TrimSpace(request.SourceURI),
		SourceLabel: strings.TrimSpace(request.SourceLabel),
		ContentHash: contentHash,
	}
	return s.repo.Create(memory)
}

func (s *service) Update(id uuid.UUID, request UpdateRequest) (*models.ContextMemory, error) {
	memory, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if request.ProjectKey != "" {
		memory.ProjectKey = strings.TrimSpace(request.ProjectKey)
	}
	if request.Kind != "" {
		memory.Kind = strings.TrimSpace(request.Kind)
	}
	if request.Content != "" {
		memory.Content = strings.TrimSpace(request.Content)
	}
	if request.Summary != "" {
		memory.Summary = compactSummary(request.Summary)
	} else {
		memory.Summary = compactSummary(memory.Content)
	}
	if request.Tags != nil {
		memory.Tags = joinTags(request.Tags)
	}
	if request.Confidence > 0 {
		memory.Confidence = normalizeConfidence(request.Confidence)
	}
	if request.SourceURI != "" {
		memory.SourceURI = strings.TrimSpace(request.SourceURI)
	}
	if request.SourceLabel != "" {
		memory.SourceLabel = strings.TrimSpace(request.SourceLabel)
	}
	if request.Archived != nil {
		memory.Archived = *request.Archived
	}
	memory.ContentHash = hashContent(memory.ProjectKey, memory.Kind, memory.Content)
	return s.repo.Update(memory)
}

func (s *service) FindAll(projectKey string, includeArchived bool) ([]models.ContextMemory, error) {
	return s.repo.FindAll(projectKey, includeArchived)
}

func (s *service) FindByID(id uuid.UUID) (*models.ContextMemory, error) {
	return s.repo.FindByID(id)
}

func (s *service) Archive(id uuid.UUID, archived bool) (*models.ContextMemory, error) {
	memory, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	memory.Archived = archived
	return s.repo.Update(memory)
}

func (s *service) Delete(id uuid.UUID) error {
	return s.repo.Delete(id)
}

func (s *service) Retrieve(request RetrieveRequest) (*RetrieveResult, error) {
	limit := request.Limit
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	memories, err := s.repo.FindAll(strings.TrimSpace(request.ProjectKey), false)
	if err != nil {
		return nil, err
	}

	ranked := make([]RankedMemory, 0, len(memories))
	for _, memory := range memories {
		score, explanation := scoreMemory(memory, request)
		if score <= 0.12 {
			continue
		}
		ranked = append(ranked, RankedMemory{
			Memory:      memory,
			Score:       math.Round(score*1000) / 1000,
			Explanation: explanation,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	now := time.Now().UTC()
	for i := range ranked {
		ranked[i].Memory.LastUsedAt = &now
		updated := ranked[i].Memory
		if saved, errUpdate := s.repo.Update(&updated); errUpdate == nil {
			ranked[i].Memory = *saved
		}
	}

	return &RetrieveResult{
		Query:       request.Query,
		ProjectKey:  request.ProjectKey,
		UsedContext: ranked,
		Explanation: fmt.Sprintf("Retrieved %d relevant memories from %d stored candidates; low-scoring unrelated memories were not loaded.", len(ranked), len(memories)),
	}, nil
}

func (s *service) mergeExact(existing *models.ContextMemory, request CreateRequest) (*models.ContextMemory, error) {
	existing.Confidence = math.Max(existing.Confidence, normalizeConfidence(request.Confidence))
	existing.Tags = joinTags(mergeTags(splitTags(existing.Tags), request.Tags))
	if existing.SourceURI == "" {
		existing.SourceURI = request.SourceURI
	}
	if existing.SourceLabel == "" {
		existing.SourceLabel = request.SourceLabel
	}
	if request.Summary != "" {
		existing.Summary = compactSummary(request.Summary)
	}
	existing.Archived = false
	return s.repo.Update(existing)
}

func (s *service) mergeSimilar(existing *models.ContextMemory, request CreateRequest) (*models.ContextMemory, error) {
	existing.Content = compactMergedContent(existing.Content, request.Content)
	existing.Summary = compactSummary(existing.Content)
	existing.Confidence = math.Max(existing.Confidence, normalizeConfidence(request.Confidence)-0.05)
	existing.Tags = joinTags(mergeTags(splitTags(existing.Tags), request.Tags))
	if existing.SourceURI == "" {
		existing.SourceURI = request.SourceURI
	}
	if existing.SourceLabel == "" {
		existing.SourceLabel = request.SourceLabel
	}
	existing.ContentHash = hashContent(existing.ProjectKey, existing.Kind, existing.Content)
	existing.Archived = false
	return s.repo.Update(existing)
}

func scoreMemory(memory models.ContextMemory, request RetrieveRequest) (float64, string) {
	queryTokens := tokenSet(request.Query)
	memoryTokens := tokenSet(memory.Content + " " + memory.Summary + " " + memory.Tags)
	relevance := overlapScore(queryTokens, memoryTokens)
	projectMatch := 0.0
	if request.ProjectKey != "" && request.ProjectKey == memory.ProjectKey {
		projectMatch = 0.2
	}
	recency := recencyScore(memory.UpdatedAt)
	confidence := normalizeConfidence(memory.Confidence) * 0.25
	score := relevance*0.45 + confidence + recency*0.10 + projectMatch

	parts := []string{
		fmt.Sprintf("relevance %.2f", relevance),
		fmt.Sprintf("confidence %.2f", memory.Confidence),
		fmt.Sprintf("recency %.2f", recency),
	}
	if projectMatch > 0 {
		parts = append(parts, "same project")
	}
	return score, strings.Join(parts, ", ")
}

func hashContent(projectKey, kind, content string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strings.ToLower(projectKey + "|" + kind + "|" + normalizeText(content))))
	return fmt.Sprintf("%x", hash.Sum64())
}

func compactSummary(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 360 {
		return value
	}
	return value[:357] + "..."
}

func compactMergedContent(existing, incoming string) string {
	merged := strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	if incoming == "" || strings.Contains(strings.ToLower(merged), strings.ToLower(incoming)) {
		return compactLongText(merged)
	}
	return compactLongText(merged + "\n" + incoming)
}

func compactLongText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 1800 {
		return value
	}
	return value[:1797] + "..."
}

func similarity(left, right string) float64 {
	leftSet := tokenSet(left)
	rightSet := tokenSet(right)
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	for token := range leftSet {
		if rightSet[token] {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	return float64(intersection) / float64(union)
}

func overlapScore(queryTokens, memoryTokens map[string]bool) float64 {
	if len(queryTokens) == 0 {
		return 0.2
	}
	matches := 0
	for token := range queryTokens {
		if memoryTokens[token] {
			matches++
		}
	}
	return float64(matches) / float64(len(queryTokens))
}

func recencyScore(value time.Time) float64 {
	days := time.Since(value).Hours() / 24
	if days <= 1 {
		return 1
	}
	if days >= 90 {
		return 0.05
	}
	return 1 - (days / 100)
}

func tokenSet(value string) map[string]bool {
	set := map[string]bool{}
	for _, token := range strings.Fields(normalizeText(value)) {
		if len(token) < 3 {
			continue
		}
		set[token] = true
	}
	return set
}

func normalizeText(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(",", " ", ".", " ", ";", " ", ":", " ", "/", " ", "\\", " ", "\n", " ", "\t", " ", "(", " ", ")", " ")
	return replacer.Replace(value)
}

func normalizeConfidence(value float64) float64 {
	if value <= 0 {
		return 0.7
	}
	if value > 1 {
		return 1
	}
	return value
}

func joinTags(tags []string) string {
	return strings.Join(mergeTags(nil, tags), ",")
}

func splitTags(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func mergeTags(existing, incoming []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, tag := range append(existing, incoming...) {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
