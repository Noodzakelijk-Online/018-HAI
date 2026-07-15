package authentication

import (
	"automation-hub-idp/internal/app/config"
	"automation-hub-idp/internal/app/dto"
	"automation-hub-idp/internal/app/models"
	"automation-hub-idp/internal/app/utils"
	"errors"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRegisterRejectsInvalidInputBeforeHashing(t *testing.T) {
	svc := &service{}

	_, err := svc.Register(dto.UserDTO{Email: "not-an-email", Password: "local-passphrase-2026"})
	require.ErrorIs(t, err, ErrRegistrationEmailInvalid)

	_, err = svc.Register(dto.UserDTO{Email: "operator@example.com", Password: "short-pass"})
	require.ErrorIs(t, err, ErrRegistrationPasswordWeak)
}

func TestRefreshTokenUsesRefreshTokenExpiration(t *testing.T) {
	setupAuthConfig(t)

	userID := uuid.New()
	svc := &service{
		userService:      &fakeUserService{userByID: &models.User{ID: userID, Role: "owner"}},
		blockListService: fakeBlockListService{},
		logger:           noopLogger{},
		jwtSecret:        "test-secret",
	}

	refreshToken, refreshUUID, refreshExp, err := svc.generateRefreshToken(userID)
	require.NoError(t, err)

	tokenDetails, err := svc.RefreshToken(refreshToken)
	require.NoError(t, err)
	require.NotEmpty(t, tokenDetails.AccessToken)
	require.Equal(t, refreshToken, tokenDetails.RefreshToken)
	require.Equal(t, refreshUUID, tokenDetails.RefreshUUID)
	require.Equal(t, refreshExp, tokenDetails.RtExpires)
	_, claims, err := svc.parseAndValidateToken(tokenDetails.AccessToken)
	require.NoError(t, err)
	require.Equal(t, "owner", claims["role"])
}

func TestGenerateAccessTokenNormalizesUnknownRole(t *testing.T) {
	setupAuthConfig(t)
	svc := &service{logger: noopLogger{}, jwtSecret: "test-secret"}
	token, _, err := svc.generateAccessToken(uuid.New(), "unexpected", uuid.New().String(), time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)
	_, claims, err := svc.parseAndValidateToken(token)
	require.NoError(t, err)
	require.Equal(t, "operator", claims["role"])
}

func TestLogoutRejectsTokenWithoutUserID(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"access_uuid":  uuid.New().String(),
		"refresh_uuid": uuid.New().String(),
		"refresh_exp":  time.Now().Add(time.Hour).Unix(),
		"exp":          time.Now().Add(time.Minute).Unix(),
	})
	tokenString, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	svc := &service{
		blockListService: fakeBlockListService{},
		logger:           noopLogger{},
		jwtSecret:        "test-secret",
	}

	err = svc.Logout(tokenString)
	require.EqualError(t, err, "user ID not found in the token")
}

func TestChangePasswordPassesPlaintextPasswordToUserService(t *testing.T) {
	userID := uuid.New()
	userService := &fakeUserService{
		userByID: &models.User{
			ID:                 userID,
			Email:              "user@example.com",
			Password:           "old-hash",
			ResetPasswordToken: "stale-token",
			ResetTokenExpires:  ptrTime(time.Now().Add(time.Hour)),
		},
	}
	svc := &service{
		userService: userService,
		logger:      noopLogger{},
		jwtSecret:   "test-secret",
	}

	err := svc.ChangePassword(signedAccessToken(t, userID, "test-secret"), "new-password")
	require.NoError(t, err)
	require.Equal(t, "new-password", userService.updatedPassword)
	require.NotNil(t, userService.updatedUser)
	require.Empty(t, userService.updatedUser.ResetPasswordToken)
	require.Nil(t, userService.updatedUser.ResetTokenExpires)
}

func TestConfirmPasswordResetRejectsEmptyTokenBeforeLookup(t *testing.T) {
	userService := &fakeUserService{}
	svc := &service{
		userService: userService,
		logger:      noopLogger{},
	}

	err := svc.ConfirmPasswordReset("   ", "new-password")
	require.EqualError(t, err, "invalid token")
	require.Empty(t, userService.lookupResetToken)
}

func TestConfirmPasswordResetRejectsWeakPasswordBeforeLookup(t *testing.T) {
	userService := &fakeUserService{}
	svc := &service{
		userService: userService,
		logger:      noopLogger{},
	}

	err := svc.ConfirmPasswordReset("reset-token", "short-pass")
	require.ErrorIs(t, err, ErrRegistrationPasswordWeak)
	require.Empty(t, userService.lookupResetToken)
}

func TestConfirmPasswordResetUpdatesPasswordAndClearsToken(t *testing.T) {
	userID := uuid.New()
	userService := &fakeUserService{
		userByResetToken: &models.User{
			ID:                 userID,
			Email:              "user@example.com",
			Password:           "old-hash",
			ResetPasswordToken: "reset-token",
			ResetTokenExpires:  ptrTime(time.Now().Add(time.Hour)),
		},
	}
	svc := &service{
		userService: userService,
		logger:      noopLogger{},
	}

	err := svc.ConfirmPasswordReset("reset-token", "new-password")
	require.NoError(t, err)
	require.Equal(t, "reset-token", userService.lookupResetToken)
	require.Equal(t, "new-password", userService.updatedPassword)
	require.NotNil(t, userService.updatedUser)
	require.Empty(t, userService.updatedUser.ResetPasswordToken)
	require.Nil(t, userService.updatedUser.ResetTokenExpires)
}

func setupAuthConfig(t *testing.T) {
	t.Helper()
	t.Setenv("LOGGER_TOPIC", "logs")
	t.Setenv("MAIL_TOPIC", "mail")
	t.Setenv("BROKERS_ADDR", "localhost:9092")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_NAME", "automation_hub")
	t.Setenv("PASSWORD_RESET_TOPIC", "password-reset")
	t.Setenv("ACCOUNT_BLOCKED_TOPIC", "account-blocked")
	t.Setenv("ACCOUNT_CREATED_TOPIC", "account-created")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("BLOCKING_TIME_EXPONENTIATION_BASIS", "1")
	t.Setenv("MAX_LOGIN_ATTEMPTS_BEFORE_BLOCK", "3")
	require.NoError(t, config.Setup())
}

func signedAccessToken(t *testing.T, userID uuid.UUID, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":      userID.String(),
		"access_uuid":  uuid.New().String(),
		"refresh_uuid": uuid.New().String(),
		"refresh_exp":  time.Now().Add(time.Hour).Unix(),
		"exp":          time.Now().Add(time.Minute).Unix(),
	})
	tokenString, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return tokenString
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

type fakeUserService struct {
	userByID         *models.User
	userByEmail      *models.User
	userByResetToken *models.User
	lookupResetToken string
	updatedPassword  string
	updatedUser      *models.User
}

func (f *fakeUserService) CreateUser(user models.User) (*models.User, error) {
	return &user, nil
}

func (f *fakeUserService) GetUserByID(id uuid.UUID) (*models.User, error) {
	if f.userByID == nil {
		return nil, errors.New("user not found")
	}
	return f.userByID, nil
}

func (f *fakeUserService) GetUserByEmail(email string) (*models.User, error) {
	if f.userByEmail == nil {
		return nil, errors.New("user not found")
	}
	return f.userByEmail, nil
}

func (f *fakeUserService) GetUserByResetToken(token string) (*models.User, error) {
	f.lookupResetToken = token
	if f.userByResetToken == nil {
		return nil, errors.New("user not found")
	}
	return f.userByResetToken, nil
}

func (f *fakeUserService) UpdateUser(user models.User) (*models.User, error) {
	f.updatedUser = &user
	return &user, nil
}

func (f *fakeUserService) DeleteUser(id uuid.UUID) error {
	return nil
}

func (f *fakeUserService) GetAllUsers(p *utils.Pagination) ([]*models.User, error) {
	return nil, nil
}

func (f *fakeUserService) UpdatePassword(id uuid.UUID, newPassword string) error {
	f.updatedPassword = newPassword
	return nil
}

type fakeBlockListService struct{}

func (fakeBlockListService) AddToBlockList(jwtUUID string, expirationTime time.Duration) error {
	return nil
}

func (fakeBlockListService) IsInBlockList(jwtUUID string) (bool, error) {
	return false, nil
}

type noopLogger struct{}

func (noopLogger) Info(message string, args ...interface{})  {}
func (noopLogger) Error(message string, args ...interface{}) {}
func (noopLogger) Warn(message string, args ...interface{})  {}
func (noopLogger) Debug(message string, args ...interface{}) {}

var _ IService = (*service)(nil)
