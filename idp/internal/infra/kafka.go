package infra

import (
	"automation-hub-idp/internal/app/config"
	"fmt"
	"regexp"
	"strings"

	"github.com/IBM/sarama"
)

var validKafkaClientID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func NewKafkaProducer(brokers []string, client string) (sarama.SyncProducer, error) {
	return newKafkaProducer(brokers, client, sarama.NewSyncProducer)
}

type kafkaProducerFactory func([]string, *sarama.Config) (sarama.SyncProducer, error)

func newKafkaProducer(brokers []string, client string, factory kafkaProducerFactory) (sarama.SyncProducer, error) {
	if factory == nil {
		return nil, fmt.Errorf("Kafka producer factory is required")
	}

	cleanBrokers := make([]string, 0, len(brokers))
	seenBrokers := make(map[string]struct{}, len(brokers))
	for _, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if broker == "" {
			continue
		}
		if _, duplicate := seenBrokers[broker]; duplicate {
			continue
		}
		seenBrokers[broker] = struct{}{}
		cleanBrokers = append(cleanBrokers, broker)
	}
	if len(cleanBrokers) == 0 {
		return nil, fmt.Errorf("at least one Kafka broker is required")
	}
	client = strings.TrimSpace(client)
	if client == "" {
		return nil, fmt.Errorf("Kafka client ID is required")
	}
	if !validKafkaClientID.MatchString(client) {
		return nil, fmt.Errorf("Kafka client ID %q contains invalid characters; use only letters, numbers, dot, underscore, or hyphen", client)
	}

	producerConfig := sarama.NewConfig()
	producerConfig.ClientID = client
	producerConfig.Net.MaxOpenRequests = 1
	producerConfig.Producer.Idempotent = true
	producerConfig.Producer.RequiredAcks = sarama.WaitForAll
	producerConfig.Producer.Return.Successes = true
	producerConfig.Producer.Return.Errors = true
	if err := producerConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Kafka producer configuration: %w", err)
	}

	producer, err := factory(cleanBrokers, producerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}
	if producer == nil {
		return nil, fmt.Errorf("failed to create Kafka producer: producer factory returned nil")
	}

	return producer, nil
}

func GetDefaultKafkaProducer() (sarama.SyncProducer, error) {
	if config.KafkaConfig == nil {
		return nil, fmt.Errorf("Kafka configuration is not initialized")
	}
	return NewKafkaProducer(config.KafkaConfig.BrokersAddr, config.KafkaConfig.ClientID)
}
