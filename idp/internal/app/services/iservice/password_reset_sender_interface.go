package iservice

import "time"

// PasswordResetSender sends a reset code through an operator-configured,
// private recovery channel. Implementations must not publish the code to a
// shared event stream.
type PasswordResetSender interface {
	Configured() bool
	SendPasswordReset(email, resetToken string, expiresAt time.Time) error
}
