package infra

import (
	"automation-hub-idp/internal/app/models"
	"automation-hub-idp/internal/app/utils"
	"errors"
	"os"
	"gorm.io/gorm"
)

func RunMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.User{}); err != nil {
		return err
	}
	return nil
}

func SeedDatabase(db *gorm.DB) error {
	hasher := utils.DefaultBcryptHasher()
	defaultPassword := firstNonEmpty(os.Getenv("FIRST_RUN_ADMIN_PASSWORD"), "ChangeMe123!")
	defaultEmail := firstNonEmpty(os.Getenv("FIRST_RUN_ADMIN_EMAIL"), "noodzakelijkonline@gmail.com")
	var user models.User
	err := db.Where("Email = ?", defaultEmail).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			hashedPassword, err := hasher.Hash(defaultPassword)
			if err != nil {
				return err
			}
			adminUser := models.User{
				Email:       defaultEmail,
				Password:    hashedPassword,
				FirstAccess: false,
			}
			if err := db.Create(&adminUser).Error; err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
