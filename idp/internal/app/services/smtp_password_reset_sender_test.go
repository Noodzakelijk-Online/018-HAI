package services

import (
	"net"
	"strconv"
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

func TestSMTPPasswordResetSenderTimesOutWhenServerDoesNotGreet(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()

	sender := NewSMTPPasswordResetSender("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port), "", "", "operator@example.com", false)
	sender.timeout = 50 * time.Millisecond

	started := time.Now()
	err = sender.SendPasswordReset("recipient@example.com", "reset-code", time.Now().Add(time.Hour))
	require.ErrorContains(t, err, "connect to smtp server")
	require.Less(t, time.Since(started), 500*time.Millisecond)

	select {
	case connection := <-accepted:
		_ = connection.Close()
	default:
	}
}
