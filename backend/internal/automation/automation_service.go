package automation

import (
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/events"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/util"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"image"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type HealthResult struct {
	AutomationID        uuid.UUID `json:"automationId"`
	Status              string    `json:"status"`
	CheckedAt           time.Time `json:"checkedAt"`
	LatencyMs           int64     `json:"latencyMs"`
	FailureReason       string    `json:"failureReason,omitempty"`
	ConsecutiveFailures int       `json:"consecutiveFailures"`
}

type HealthSummary struct {
	Total     int       `json:"total"`
	Healthy   int       `json:"healthy"`
	Warning   int       `json:"warning"`
	Degraded  int       `json:"degraded"`
	Broken    int       `json:"broken"`
	Unknown   int       `json:"unknown"`
	CheckedAt time.Time `json:"checkedAt"`
}

type LaunchResult struct {
	AutomationID uuid.UUID `json:"automationId"`
	LaunchType   string    `json:"launchType"`
	Target       string    `json:"target"`
	LaunchedAt   time.Time `json:"launchedAt"`
}

type DiagnosticResult struct {
	AutomationID      uuid.UUID                      `json:"automationId"`
	Name              string                         `json:"name"`
	Status            string                         `json:"status"`
	LaunchTarget      string                         `json:"launchTarget"`
	HealthCheckTarget string                         `json:"healthCheckTarget"`
	RoutePath         string                         `json:"routePath"`
	Host              string                         `json:"host"`
	Port              int                            `json:"port"`
	LastCheckedAt     *time.Time                     `json:"lastCheckedAt,omitempty"`
	LastSuccessAt     *time.Time                     `json:"lastSuccessAt,omitempty"`
	LastFailureAt     *time.Time                     `json:"lastFailureAt,omitempty"`
	LastFailureReason string                         `json:"lastFailureReason,omitempty"`
	Checks            map[string]string              `json:"checks"`
	RecentEvents      []models.AutomationHealthEvent `json:"recentEvents"`
}

type Service interface {
	FindByID(id uuid.UUID) (*models.Automation, error)
	Create(automation *models.Automation) (*models.Automation, error)
	Update(automation *models.Automation) (*models.Automation, error)
	Delete(id uuid.UUID) error
	FindAll() ([]*models.Automation, error)
	SwapOrder(id1 uuid.UUID, id2 uuid.UUID) error
	RunHealthCheck(id uuid.UUID) (*HealthResult, error)
	HealthSummary() (*HealthSummary, error)
	Launch(id uuid.UUID) (*LaunchResult, error)
	Diagnostics(id uuid.UUID) (*DiagnosticResult, error)
}

type service struct {
	repo      Repository
	publisher events.Publisher
}

func NewService(repo Repository, publisher events.Publisher) Service {
	return &service{
		repo:      repo,
		publisher: publisher,
	}
}

func DefaultService() Service {
	repo := DefaultRepository()
	pub := events.DefaultPublisher()
	return NewService(repo, *pub)
}

func (s *service) FindByID(id uuid.UUID) (*models.Automation, error) {
	return s.repo.FindByID(id)
}

func (s *service) Create(automation *models.Automation) (*models.Automation, error) {
	automation.ID = uuid.UUID{} // reset ID

	if automation.ImageFile != nil {
		newFileName, err := s.processImageFile(automation.ImageFile)
		if err != nil {
			return nil, err
		}
		automation.Image = newFileName
	}

	maxPosition, err := s.repo.MaxPosition()
	if err != nil {
		return nil, err
	}
	automation.Position = maxPosition + 1

	err = s.ensureUniqueURLPath(automation)
	if err != nil {
		return nil, err
	}
	s.applyAutomationDefaults(automation)

	if err := automation.Validate(); err != nil {
		return nil, err
	}

	automationCreated, err := s.repo.Create(automation)
	if err != nil {
		return nil, err
	}
	event := &events.AutomationEvent{
		Type:       events.CreateEvent,
		Automation: automationCreated,
	}
	err = s.publisher.Publish(event)
	if err != nil {
		log.Printf("Failed to publish create event to Kafka: %v", err)
		return nil, err
	}
	return automationCreated, nil
}

func (s *service) Update(automation *models.Automation) (*models.Automation, error) {
	currentAutomation, err := s.repo.FindByID(automation.ID)
	if err != nil {
		return nil, err
	}

	automation.Position = currentAutomation.Position
	automation.LastCheckedAt = currentAutomation.LastCheckedAt
	automation.LastSuccessAt = currentAutomation.LastSuccessAt
	automation.LastFailureAt = currentAutomation.LastFailureAt
	automation.LastFailureReason = currentAutomation.LastFailureReason
	automation.ConsecutiveFailures = currentAutomation.ConsecutiveFailures
	automation.AverageLatencyMs = currentAutomation.AverageLatencyMs
	automation.LastLaunchAt = currentAutomation.LastLaunchAt

	if automation.ImageFile != nil {
		newFileName, errIf := s.processImageFile(automation.ImageFile)
		log.Printf("Image processed and saved as: %s", newFileName)
		if errIf != nil {
			return nil, errIf
		}
		if ok := s.deleteImage(currentAutomation.Image); ok != nil {
			return nil, ok
		}
		automation.Image = newFileName
	} else if automation.RemoveImage {
		if noDeleted := s.deleteImage(currentAutomation.Image); noDeleted != nil {
			return nil, noDeleted
		}
		automation.Image = ""
	} else {
		automation.Image = currentAutomation.Image
	}
	var oldUrlPath string
	if currentAutomation.Name != automation.Name {
		oldUrlPath = currentAutomation.URLPath
		err = s.ensureUniqueURLPath(automation)
		if err != nil {
			return nil, err
		}
	} else {
		oldUrlPath = currentAutomation.URLPath
		automation.URLPath = currentAutomation.URLPath
	}

	s.applyAutomationDefaults(automation)
	if errValidate := automation.Validate(); errValidate != nil {
		return nil, errValidate
	}

	automationUpdated, err := s.repo.Update(automation)
	automationUpdated.OldUrlPath = oldUrlPath

	event := &events.AutomationEvent{
		Type:       events.UpdateEvent,
		Automation: automationUpdated,
	}

	err = s.publisher.Publish(event)
	if err != nil {
		log.Printf("Failed to publish update event to Kafka: %v", err)
		return nil, err
	}

	return automationUpdated, nil
}

func (s *service) Delete(id uuid.UUID) error {
	automation, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}

	err = s.repo.Delete(id)
	if err != nil {
		return err
	}

	event := &events.AutomationEvent{
		Type:       events.DeleteEvent,
		Automation: automation,
	}

	err = s.publisher.Publish(event)
	if err != nil {
		log.Printf("Failed to publish delete event to Kafka: %v", err)
		return err
	}

	return nil
}

func (s *service) FindAll() ([]*models.Automation, error) {
	return s.repo.FindAll()
}

func (s *service) SwapOrder(id1 uuid.UUID, id2 uuid.UUID) error {
	return s.repo.Transaction(func(tx *gorm.DB) error {
		automation1, err := s.repo.FindByID(id1)
		if err != nil {
			return err
		}
		automation2, err := s.repo.FindByID(id2)
		if err != nil {
			return err
		}

		pos1 := automation1.Position
		pos2 := automation2.Position

		maxPosition, err := s.repo.MaxPosition()
		if err != nil {
			return err
		}
		tempPosition := maxPosition + 1

		automation1.Position = tempPosition
		if err := tx.Save(automation1).Error; err != nil {
			return err
		}

		automation2.Position = pos1
		if err := tx.Save(automation2).Error; err != nil {
			return err
		}

		automation1.Position = pos2
		if err := tx.Save(automation1).Error; err != nil {
			return err
		}

		return nil
	})
}

func (s *service) RunHealthCheck(id uuid.UUID) (*HealthResult, error) {
	automation, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	status := "healthy"
	failureReason := ""
	target := ""

	s.applyAutomationDefaults(automation)

	checkType := strings.ToLower(automation.HealthCheckType)
	switch checkType {
	case "tcp":
		target = fmt.Sprintf("%s:%d", automation.Host, automation.Port)
		conn, errDial := net.DialTimeout("tcp", target, 5*time.Second)
		if errDial != nil {
			status = classifyFailure(automation.ConsecutiveFailures + 1)
			failureReason = errDial.Error()
		} else {
			_ = conn.Close()
		}
	case "manual", "disabled":
		status = "unknown"
		failureReason = "automatic health checks are disabled for this automation"
	default:
		target = automation.HealthCheckURL
		if target == "" {
			target = fmt.Sprintf("http://%s:%d", automation.Host, automation.Port)
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, errGet := client.Get(target)
		if errGet != nil {
			status = classifyFailure(automation.ConsecutiveFailures + 1)
			failureReason = errGet.Error()
		} else {
			defer resp.Body.Close()
			expected := automation.ExpectedHTTPStatus
			if expected == 0 {
				expected = http.StatusOK
			}
			if resp.StatusCode != expected {
				status = classifyFailure(automation.ConsecutiveFailures + 1)
				failureReason = fmt.Sprintf("unexpected HTTP status: got %d, expected %d", resp.StatusCode, expected)
			}
		}
	}

	latency := time.Since(started).Milliseconds()
	checkedAt := time.Now().UTC()
	automation.LastCheckedAt = &checkedAt
	automation.AverageLatencyMs = latency
	if status == "healthy" {
		automation.LastSuccessAt = &checkedAt
		automation.LastFailureReason = ""
		automation.ConsecutiveFailures = 0
	} else if status == "unknown" {
		automation.LastFailureReason = failureReason
	} else {
		automation.LastFailureAt = &checkedAt
		automation.LastFailureReason = failureReason
		automation.ConsecutiveFailures++
	}
	automation.Status = status

	if _, errUpdate := s.repo.Update(automation); errUpdate != nil {
		return nil, errUpdate
	}

	// Persist the check as a health-history event. A history write failure
	// must not fail the check itself, so it is only logged.
	event := &models.AutomationHealthEvent{
		AutomationID:        automation.ID,
		Status:              status,
		CheckType:           checkType,
		Target:              target,
		LatencyMs:           latency,
		FailureReason:       failureReason,
		ConsecutiveFailures: automation.ConsecutiveFailures,
		CheckedAt:           checkedAt,
	}
	if errEvent := s.repo.SaveHealthEvent(event); errEvent != nil {
		log.Printf("Failed to persist health event for automation %s: %v", automation.ID, errEvent)
	}

	return &HealthResult{
		AutomationID:        automation.ID,
		Status:              status,
		CheckedAt:           checkedAt,
		LatencyMs:           latency,
		FailureReason:       failureReason,
		ConsecutiveFailures: automation.ConsecutiveFailures,
	}, nil
}

func (s *service) HealthSummary() (*HealthSummary, error) {
	automations, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	summary := &HealthSummary{CheckedAt: time.Now().UTC()}
	summary.Total = len(automations)
	for _, automation := range automations {
		switch strings.ToLower(automation.Status) {
		case "healthy":
			summary.Healthy++
		case "warning":
			summary.Warning++
		case "degraded":
			summary.Degraded++
		case "broken":
			summary.Broken++
		default:
			summary.Unknown++
		}
	}
	return summary, nil
}

func (s *service) Launch(id uuid.UUID) (*LaunchResult, error) {
	automation, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	s.applyAutomationDefaults(automation)
	launchedAt := time.Now().UTC()
	automation.LastLaunchAt = &launchedAt
	if _, errUpdate := s.repo.Update(automation); errUpdate != nil {
		return nil, errUpdate
	}
	return &LaunchResult{
		AutomationID: automation.ID,
		LaunchType:   automation.LaunchType,
		Target:       automation.LaunchTarget,
		LaunchedAt:   launchedAt,
	}, nil
}

func (s *service) Diagnostics(id uuid.UUID) (*DiagnosticResult, error) {
	automation, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	s.applyAutomationDefaults(automation)
	checks := map[string]string{
		"launchTargetConfigured": boolStatus(automation.LaunchTarget != ""),
		"healthCheckConfigured":  boolStatus(automation.HealthCheckType != ""),
		"routePathConfigured":    boolStatus(automation.RoutePath != "" || automation.URLPath != ""),
		"hostConfigured":         boolStatus(automation.Host != ""),
		"portConfigured":         boolStatus(automation.Port > 0 && automation.Port <= 65535),
		"dependencyNotesPresent": boolStatus(automation.DependencyNotes != ""),
	}
	recentEvents, errEvents := s.repo.FindHealthEvents(automation.ID, 10)
	if errEvents != nil {
		log.Printf("Failed to load health history for automation %s: %v", automation.ID, errEvents)
		recentEvents = []models.AutomationHealthEvent{}
	}

	return &DiagnosticResult{
		AutomationID:      automation.ID,
		Name:              automation.Name,
		Status:            automation.Status,
		LaunchTarget:      automation.LaunchTarget,
		HealthCheckTarget: automation.HealthCheckURL,
		RoutePath:         firstNonEmpty(automation.RoutePath, automation.URLPath),
		Host:              automation.Host,
		Port:              automation.Port,
		LastCheckedAt:     automation.LastCheckedAt,
		LastSuccessAt:     automation.LastSuccessAt,
		LastFailureAt:     automation.LastFailureAt,
		LastFailureReason: automation.LastFailureReason,
		Checks:            checks,
		RecentEvents:      recentEvents,
	}, nil
}

func (s *service) processImageFile(file *multipart.FileHeader) (string, error) {
	log.Println("Starting processImageFile function")
	if file.Size > config.AppConfig.ImageMaxSize {
		return "", fmt.Errorf("image is too large (%d). Max size is %d Mb", file.Size, config.AppConfig.ImageMaxSize)
	}

	ext := filepath.Ext(file.Filename)
	fmt.Printf("Filename: %s, Extracted Extension: %s\n", file.Filename, ext)

	if !contains(config.AppConfig.ImageExtensions, ext) {
		return "", fmt.Errorf("invalid image extension. Allowed extensions are: %v", config.AppConfig.ImageExtensions)
	}

	src, err := file.Open()
	if err != nil {
		log.Printf("Failed to open the file: %v", err)
		return "", err
	}
	defer src.Close()
	log.Println("After opening source file")

	buffer := make([]byte, 512)
	_, err = src.Read(buffer)
	if err != nil {
		return "", err
	}

	log.Println("After reading buffer")

	fileType := http.DetectContentType(buffer)
	if !strings.HasPrefix(fileType, "image/") {
		return "", fmt.Errorf("file is not an image")
	}
	mimeSuffix := strings.TrimPrefix(fileType, "image/")
	if !contains(config.AppConfig.ImageExtensions, "."+mimeSuffix) {
		return "", fmt.Errorf("mismatch between file extension and MIME type")
	}

	_, err = src.Seek(0, 0)
	if err != nil {
		return "", err
	}

	log.Println("After seeking to start of source file")

	_, _, err = image.Decode(src)
	if err != nil {
		//return "", fmt.Errorf("corrupted image: %v", err)
	}

	_, err = src.Seek(0, 0)
	if err != nil {
		return "", err
	}

	newFileName := uuid.New().String() + ext
	fullPath := config.AppConfig.ImageSaveDir + "/" + newFileName
	dst, err := os.Create(fullPath)
	if err != nil {
		fmt.Printf("Failed to create file %s: %v", dst.Name(), err)
		return "", err
	}
	defer dst.Close()
	fmt.Printf("Buffer content: %x\n", buffer[:100]) // Print first 100 bytes
	log.Printf("File path: %s", fullPath)

	log.Println("Before copying file")

	n, err := io.Copy(dst, src)
	if err != nil {
		log.Printf("Failed to copy file: %v", err)
		return "", err
	}
	log.Printf("Copied %d bytes to %s", n, dst.Name())
	return newFileName, nil
}

func (s *service) deleteImage(imageName string) error {
	if imageName == "" {
		return nil
	}
	imagePath := config.AppConfig.ImageSaveDir + "/" + imageName
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(imagePath)
}

func contains(slice []string, str string) bool {
	str = strings.ToLower(str)
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func (s *service) ensureUniqueURLPath(automation *models.Automation) error {
	baseURLPath := util.GenerateURLPath(automation.Name)
	uniqueURLPath := baseURLPath
	counter := 0

	for {
		existingAutomation, err := s.repo.GetByURLPath(uniqueURLPath)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		if existingAutomation == nil || existingAutomation.ID == automation.ID {
			break
		}

		counter++
		uniqueURLPath = fmt.Sprintf("%s-%d", baseURLPath, counter)
	}

	automation.URLPath = uniqueURLPath
	return nil
}

func (s *service) applyAutomationDefaults(automation *models.Automation) {
	if automation.LaunchType == "" {
		automation.LaunchType = "browser_url"
	}
	if automation.HealthCheckType == "" {
		automation.HealthCheckType = "http"
	}
	if automation.HealthCheckIntervalSeconds == 0 {
		automation.HealthCheckIntervalSeconds = 60
	}
	if automation.ExpectedHTTPStatus == 0 {
		automation.ExpectedHTTPStatus = http.StatusOK
	}
	if automation.Status == "" {
		automation.Status = "unknown"
	}
	if automation.RoutePath == "" {
		automation.RoutePath = automation.URLPath
	}
	if automation.LaunchTarget == "" {
		if automation.PublicURL != "" {
			automation.LaunchTarget = automation.PublicURL
		} else if automation.LocalURL != "" {
			automation.LaunchTarget = automation.LocalURL
		} else {
			automation.LaunchTarget = fmt.Sprintf("/%s", automation.URLPath)
		}
	}
	if automation.HealthCheckURL == "" && automation.Host != "" && automation.Port > 0 {
		automation.HealthCheckURL = fmt.Sprintf("http://%s:%d", automation.Host, automation.Port)
	}
}

func classifyFailure(failures int) string {
	if failures >= 3 {
		return "broken"
	}
	if failures >= 2 {
		return "degraded"
	}
	return "warning"
}

func boolStatus(value bool) string {
	if value {
		return "ok"
	}
	return "missing"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
