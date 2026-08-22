package services

import (
	"automation-hub-idp/internal/app/config"
	"automation-hub-idp/internal/app/services/iservice"
	"log"
)

// localLogger preserves audit-relevant application logs when the optional
// Kafka event bus is not enabled on a local installation.
type localLogger struct{}

func (l *localLogger) Info(message string, args ...interface{})  { l.write("INFO", message, args...) }
func (l *localLogger) Error(message string, args ...interface{}) { l.write("ERROR", message, args...) }
func (l *localLogger) Warn(message string, args ...interface{})  { l.write("WARN", message, args...) }
func (l *localLogger) Debug(message string, args ...interface{}) { l.write("DEBUG", message, args...) }

func (l *localLogger) write(level, message string, args ...interface{}) {
	log.Printf("[%s] "+message, append([]interface{}{level}, args...)...)
}

type discardMessageSender struct{}

func (discardMessageSender) Send(string, interface{}) error { return nil }

// NewDefaultLogger chooses the optional Kafka event stream only when the IDP
// was explicitly configured for it. Local installations otherwise log to the
// container's standard output, which Docker retains and exposes diagnostically.
func NewDefaultLogger() (iservice.Logger, error) {
	if config.KafkaConfig == nil {
		return &localLogger{}, nil
	}
	return NewKafkaLogger(config.KafkaConfig.BrokersAddr, config.KafkaConfig.LoggerTopic)
}

// NewDefaultMessageSender intentionally drops non-critical account events when
// Kafka is disabled. Authentication never depends on best-effort notifications.
func NewDefaultMessageSender() (iservice.MessageSender, error) {
	if config.KafkaConfig == nil {
		return discardMessageSender{}, nil
	}
	return NewKafkaMessageSender()
}
