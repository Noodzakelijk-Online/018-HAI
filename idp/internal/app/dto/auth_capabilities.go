package dto

// AuthCapabilities tells the public login page which optional authentication
// paths are truly configured on this HAI node. It contains no secrets.
type AuthCapabilities struct {
	GoogleLoginEnabled           bool `json:"googleLoginEnabled"`
	PasswordRecoveryEmailEnabled bool `json:"passwordRecoveryEmailEnabled"`
}
