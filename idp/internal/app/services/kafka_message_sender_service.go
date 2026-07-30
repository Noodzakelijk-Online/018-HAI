package services

import (
	"automation-hub-idp/internal/infra"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/IBM/sarama"
)

type KafkaMessageSender struct {
	producer sarama.SyncProducer
	mu       sync.RWMutex
	closed   bool
}

func NewKafkaMessageSender() (*KafkaMessageSender, error) {
	prod, err := infra.GetDefaultKafkaProducer()
	if err != nil {
		return nil, fmt.Errorf("create Kafka message sender: %w", err)
	}
	return &KafkaMessageSender{
		producer: prod,
	}, nil
}

func (k *KafkaMessageSender) Send(topic string, message interface{}) error {
	if k == nil {
		return fmt.Errorf("send Kafka message: producer is not configured")
	}
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.producer == nil {
		return fmt.Errorf("send Kafka message: producer is not configured")
	}
	if k.closed {
		return fmt.Errorf("send Kafka message: producer is closed")
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("send Kafka message: topic is required")
	}

	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode Kafka message for topic %q: %w", topic, err)
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msgBytes),
	}
	if _, _, err = k.producer.SendMessage(msg); err != nil {
		return fmt.Errorf("send Kafka message to topic %q: %w", topic, err)
	}
	return nil
}

func (k *KafkaMessageSender) Close() error {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.producer == nil || k.closed {
		return nil
	}
	k.closed = true
	if err := k.producer.Close(); err != nil {
		return fmt.Errorf("close Kafka message sender: %w", err)
	}
	return nil
}
