package services

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTPPasswordResetSender delivers short-lived reset codes over a standard
// STARTTLS SMTP connection. It deliberately supports port 587-style STARTTLS;
// implicit TLS (usually port 465) is not silently downgraded to plaintext.
type SMTPPasswordResetSender struct {
	host            string
	port            string
	username        string
	password        string
	from            string
	requireStartTLS bool
}

func NewSMTPPasswordResetSender(host, port, username, password, from string, requireStartTLS bool) *SMTPPasswordResetSender {
	return &SMTPPasswordResetSender{
		host:            strings.TrimSpace(host),
		port:            strings.TrimSpace(port),
		username:        strings.TrimSpace(username),
		password:        strings.TrimSpace(password),
		from:            strings.TrimSpace(from),
		requireStartTLS: requireStartTLS,
	}
}

func (s *SMTPPasswordResetSender) Configured() bool {
	if s == nil || s.host == "" || s.port == "" || s.from == "" {
		return false
	}
	port, err := strconv.Atoi(s.port)
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	if _, err := mail.ParseAddress(s.from); err != nil {
		return false
	}
	if (s.username == "") != (s.password == "") {
		return false
	}
	// This sender supports explicit STARTTLS (the common port-587 flow), not
	// implicit TLS. Refuse authenticated SMTP without it so an accidental env
	// override cannot send the mailbox credentials over a plaintext connection.
	return s.username == "" || s.requireStartTLS
}

func (s *SMTPPasswordResetSender) SendPasswordReset(email, resetToken string, expiresAt time.Time) error {
	if !s.Configured() {
		return errors.New("password reset email delivery is not configured")
	}
	parsedRecipient, err := mail.ParseAddress(email)
	if err != nil || parsedRecipient.Address != email {
		return errors.New("invalid recipient email")
	}

	client, err := smtp.Dial(net.JoinHostPort(s.host, s.port))
	if err != nil {
		return fmt.Errorf("connect to smtp server: %w", err)
	}
	defer client.Quit() //nolint:errcheck // The message result is decided before QUIT.

	if s.requireStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not support required STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("start smtp TLS: %w", err)
		}
	}
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("authenticate with smtp server: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("set smtp sender: %w", err)
	}
	if err := client.Rcpt(parsedRecipient.Address); err != nil {
		return fmt.Errorf("set smtp recipient: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open smtp message: %w", err)
	}
	message := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: HAI password reset code\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour one-time HAI password reset code is:\r\n\r\n%s\r\n\r\nIt expires at %s. If you did not request this reset, you can ignore this email.\r\n", parsedRecipient.Address, s.from, resetToken, expiresAt.UTC().Format(time.RFC1123))
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("send smtp message: %w", err)
	}
	return nil
}
