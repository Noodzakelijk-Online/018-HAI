package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMTPPasswordResetSenderRequiresCompleteConfiguration(t *testing.T) {
	sender := NewSMTPPasswordResetSender("smtp.example.com", "587", "operator@example.com", "", "operator@example.com", true)
	require.False(t, sender.Configured())

	err := sender.SendPasswordReset("recipient@example.com", "reset-code", time.Now().Add(time.Hour))
	require.EqualError(t, err, "password reset email delivery is not configured")
}

func TestSMTPPasswordResetSenderRejectsInvalidPort(t *testing.T) {
	sender := NewSMTPPasswordResetSender("smtp.example.com", "not-a-port", "operator@example.com", "app-password", "operator@example.com", true)
	require.False(t, sender.Configured())
}

func TestSMTPPasswordResetSenderValidatesRecipientBeforeConnecting(t *testing.T) {
	sender := NewSMTPPasswordResetSender("smtp.example.com", "587", "operator@example.com", "app-password", "operator@example.com", true)
	require.True(t, sender.Configured())

	err := sender.SendPasswordReset("not-an-email", "reset-code", time.Now().Add(time.Hour))
	require.EqualError(t, err, "invalid recipient email")
}
