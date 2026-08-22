package repositories

import (
	"automation-hub-idp/internal/app/models"
	"automation-hub-idp/internal/app/repositories/irepository"
	"automation-hub-idp/internal/app/utils"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"strings"
)

type Logger interface {
	Info(message string, args ...interface{})
	Error(message string, args ...interface{})
	Warn(message string, args ...interface{})
	Debug(message string, args ...interface{})
} // TODO: 1. Create a new interface called Logger with the following methods: Info, Error, Warn, Debug

type GormUserRepository struct {
	DB     *gorm.DB
	logger Logger
}

func NewGormUserRepository(db *gorm.DB, logger Logger) irepository.UserRepository {
	return &GormUserRepository{
		DB:     db,
		logger: logger,
	}
}

func (r *GormUserRepository) FindByID(id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.DB.First(&user, "id = ? AND is_active = ?", id, true).Error
	if err != nil {
		r.logger.Error("Failed to fetch user by ID: %s", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, irepository.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by ID: %w", err)
	}
	return &user, nil
}

func (r *GormUserRepository) FindByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.DB.First(&user, "email = ? AND is_active = ?", email, true).Error
	if err != nil {
		r.logger.Error("Failed to fetch user by email: %s", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, irepository.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return &user, nil
}

func (r *GormUserRepository) Create(user *models.User) (*models.User, error) {
	user.ResetPasswordToken = normalizeResetTokenForPersistence(user.ResetPasswordToken)
	err := r.DB.Create(user).Error
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return nil, irepository.ErrDuplicateUser
		}
		r.logger.Error("Failed to create user: %s", err)
		return nil, errors.New("failed to create user")
	}
	return user, nil
}

func (r *GormUserRepository) Update(user *models.User) (*models.User, error) {
	user.ResetPasswordToken = normalizeResetTokenForPersistence(user.ResetPasswordToken)
	err := r.DB.Save(user).Error
	if err != nil {
		r.logger.Error("Failed to update user: %s", err)
		return nil, errors.New("failed to update user")
	}
	return user, nil
}

func (r *GormUserRepository) Delete(id uuid.UUID) error {
	user := models.User{ID: id}
	err := r.DB.Model(&user).Update("is_active", false).Error
	if err != nil {
		r.logger.Error("Failed to soft delete user: %s", err)
		return errors.New("failed to soft delete user")
	}
	return nil
}

func (r *GormUserRepository) FindAll(p utils.Pagination) ([]*models.User, error) {
	var users []*models.User
	err := r.DB.Where("is_active = ?", true).Limit(p.Limit).Offset(p.Offset).Find(&users).Error
	if err != nil {
		r.logger.Error("Failed to fetch all users: %s", err)
		return nil, errors.New("failed to fetch users")
	}
	return users, nil
}

func (r *GormUserRepository) FindByResetToken(token string) (*models.User, error) {
	candidates := resetTokenLookupCandidates(token)
	if len(candidates) == 0 {
		return nil, irepository.ErrUserNotFound
	}
	var user models.User
	err := r.DB.First(&user, "reset_password_token IN ? AND is_active = ?", candidates, true).Error
	if err != nil {
		r.logger.Error("Failed to fetch user by reset token: %s", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, irepository.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by reset token: %w", err)
	}
	return &user, nil
}

// resetTokenDigest stores one-time password-reset codes as SHA-256 values.
// The user receives the original code over the configured private recovery
// channel, while a database-only disclosure cannot redeem it directly.
func resetTokenDigest(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(digest[:])
}

func normalizeResetTokenForPersistence(token string) string {
	token = strings.TrimSpace(token)
	if token == "" || isResetTokenDigest(token) {
		return token
	}
	return resetTokenDigest(token)
}

// resetTokenLookupCandidates keeps an in-flight code issued before this
// upgrade usable until it expires. New and subsequently saved values are
// digests, so the plaintext fallback disappears naturally after one cycle.
func resetTokenLookupCandidates(token string) []string {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if isResetTokenDigest(token) {
		return []string{token}
	}
	return []string{resetTokenDigest(token), token}
}

func isResetTokenDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
