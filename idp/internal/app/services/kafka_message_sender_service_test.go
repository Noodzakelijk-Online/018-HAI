package services

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

func TestKafkaMessageSenderSendsJSONToRequestedTopic(t *testing.T) {
	producer := mocks.NewSyncProducer(t, mocks.NewTestConfig())
	producer.ExpectSendMessageWithMessageCheckerFunctionAndSucceed(func(message *sarama.ProducerMessage) error {
		if message.Topic != "account-events" {
			return fmt.Errorf("topic = %q, want account-events", message.Topic)
		}
		payload, err := message.Value.Encode()
		if err != nil {
			return err
		}
		if string(payload) != `{"account":"robert","active":true}` {
			return fmt.Errorf("payload = %s", payload)
		}
		return nil
	})

	sender := &KafkaMessageSender{producer: producer}
	if err := sender.Send(" account-events ", map[string]interface{}{
		"account": "robert",
		"active":  true,
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if err := sender.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestKafkaMessageSenderReturnsProducerFailure(t *testing.T) {
	producer := mocks.NewSyncProducer(t, mocks.NewTestConfig())
	expected := errors.New("broker unavailable")
	producer.ExpectSendMessageAndFail(expected)

	sender := &KafkaMessageSender{producer: producer}
	err := sender.Send("account-events", struct{}{})
	if !errors.Is(err, expected) {
		t.Fatalf("Send() error = %v, want %v", err, expected)
	}
	if !strings.Contains(err.Error(), `topic "account-events"`) {
		t.Fatalf("Send() error lacks topic context: %v", err)
	}
	_ = sender.Close()
}

func TestKafkaMessageSenderRejectsUnmarshalablePayload(t *testing.T) {
	producer := mocks.NewSyncProducer(t, mocks.NewTestConfig())
	sender := &KafkaMessageSender{producer: producer}

	err := sender.Send("account-events", func() {})
	if err == nil {
		t.Fatal("Send() should reject a payload that cannot be encoded as JSON")
	}
	if !strings.Contains(err.Error(), `encode Kafka message for topic "account-events"`) {
		t.Fatalf("Send() error lacks encoding context: %v", err)
	}
	_ = sender.Close()
}

func TestKafkaMessageSenderRejectsBlankTopic(t *testing.T) {
	producer := mocks.NewSyncProducer(t, mocks.NewTestConfig())
	sender := &KafkaMessageSender{producer: producer}

	if err := sender.Send(" \t ", struct{}{}); err == nil {
		t.Fatal("Send() should reject a blank topic")
	}
	_ = sender.Close()
}

func TestKafkaMessageSenderRejectsMissingProducer(t *testing.T) {
	var nilSender *KafkaMessageSender
	if err := nilSender.Send("account-events", struct{}{}); err == nil {
		t.Fatal("Send() should reject a nil sender")
	}
	if err := (&KafkaMessageSender{}).Send("account-events", struct{}{}); err == nil {
		t.Fatal("Send() should reject a sender without a producer")
	}
}

func TestKafkaMessageSenderClosePropagatesProducerFailure(t *testing.T) {
	expected := errors.New("close failed")
	sender := &KafkaMessageSender{
		producer: closeErrorProducer{
			closeErr: expected,
		},
	}

	if err := sender.Close(); !errors.Is(err, expected) {
		t.Fatalf("Close() error = %v, want wrapped %v", err, expected)
	}
}

func TestKafkaMessageSenderCloseAllowsNilSender(t *testing.T) {
	var nilSender *KafkaMessageSender
	if err := nilSender.Close(); err != nil {
		t.Fatalf("nil sender Close() error = %v", err)
	}
	if err := (&KafkaMessageSender{}).Close(); err != nil {
		t.Fatalf("zero-value sender Close() error = %v", err)
	}
}

func TestKafkaMessageSenderRejectsSendAfterCloseAndCloseIsIdempotent(t *testing.T) {
	producer := mocks.NewSyncProducer(t, mocks.NewTestConfig())
	sender := &KafkaMessageSender{producer: producer}

	if err := sender.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := sender.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	err := sender.Send("account-events", struct{}{})
	if err == nil || !strings.Contains(err.Error(), "producer is closed") {
		t.Fatalf("Send() after Close error = %v", err)
	}
}

type closeErrorProducer struct {
	sarama.SyncProducer
	closeErr error
}

func (p closeErrorProducer) Close() error {
	return p.closeErr
}
