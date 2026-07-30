package services

import (
	"fmt"
	"testing"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

func TestKafkaLoggerSendsAcknowledgedMessageToConfiguredTopic(t *testing.T) {
	producer := mocks.NewSyncProducer(t, mocks.NewTestConfig())
	producer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(func(message *sarama.ProducerMessage) error {
		if message.Topic != "idp-logs" {
			return fmt.Errorf("topic = %q, want idp-logs", message.Topic)
		}
		payload, err := message.Value.Encode()
		if err != nil {
			return err
		}
		if string(payload) != "[INFO] started 3 workers" {
			return fmt.Errorf("payload = %q", payload)
		}
		return nil
	})
	logger, err := newKafkaLogger(producer, " idp-logs ")
	if err != nil {
		t.Fatalf("newKafkaLogger() error = %v", err)
	}

	logger.Info("started %d workers", 3)
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestKafkaLoggerRejectsMissingConfiguration(t *testing.T) {
	if _, err := newKafkaLogger(nil, "idp-logs"); err == nil {
		t.Fatal("newKafkaLogger() should reject a nil producer")
	}
	producer := mocks.NewSyncProducer(t, mocks.NewTestConfig())
	if _, err := newKafkaLogger(producer, " "); err == nil {
		t.Fatal("newKafkaLogger() should reject a blank topic")
	}
	if err := producer.Close(); err != nil {
		t.Fatalf("close producer: %v", err)
	}
}
