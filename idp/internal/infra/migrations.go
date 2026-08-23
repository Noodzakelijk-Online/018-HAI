package infra

import (
	"automation-hub-idp/internal/app/models"
	"automation-hub-idp/internal/app/utils"
	"errors"
	"gorm.io/gorm"
	"os"
	"strings"
)

func RunMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.User{}); err != nil {
		return err
	}
	// Existing installations predate signed roles. Treat those accounts as
	// operators until the configured first-run owner is promoted during seeding;
	// never upgrade every historical account to owner implicitly.
	if err := db.Model(&models.User{}).
		Where("role = '' OR role IS NULL").
		Update("role", "operator").Error; err != nil {
		return err
	}
	return nil
}

func SeedDatabase(db *gorm.DB) error {
	hasher := utils.DefaultBcryptHasher()
	defaultEmail := firstNonEmpty(os.Getenv("FIRST_RUN_ADMIN_EMAIL"), "noodzakelijkonline@gmail.com")
	var user models.User
	err := db.Where("Email = ?", defaultEmail).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			defaultPassword, err := configuredFirstRunAdminPassword()
			if err != nil {
				return err
			}
			hashedPassword, err := hasher.Hash(defaultPassword)
			if err != nil {
				return err
			}
			adminUser := models.User{
				Email:       defaultEmail,
				Password:    hashedPassword,
				Role:        "owner",
				FirstAccess: false,
			}
			if err := db.Create(&adminUser).Error; err != nil {
				return err
			}
		} else {
			return err
		}
	} else if user.Role != "owner" {
		// FIRST_RUN_ADMIN_EMAIL remains the explicit ownership source after an
		// upgrade, so an existing local administrator receives a signed owner
		// role without promoting unrelated accounts.
		if err := db.Model(&user).Update("role", "owner").Error; err != nil {
			return err
		}
	}
	return nil
}

func configuredFirstRunAdminPassword() (string, error) {
	password := strings.TrimSpace(os.Getenv("FIRST_RUN_ADMIN_PASSWORD"))
	lowerPassword := strings.ToLower(password)
	if len(password) < 12 ||
		strings.Contains(lowerPassword, "change-this") ||
		strings.Contains(lowerPassword, "changeme") ||
		strings.Contains(lowerPassword, "placeholder") ||
		strings.Contains(lowerPassword, "example") {
		return "", errors.New("FIRST_RUN_ADMIN_PASSWORD must be a non-placeholder password with at least 12 characters before the first owner account can be created")
	}
	return password, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
