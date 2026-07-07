package events

import (
	"automation-hub-backend/internal/config"
	"encoding/json"
	"log"
	"strings"

	"github.com/IBM/sarama"
)

type Publisher struct {
	producer sarama.SyncProducer
	topic    string
}

func NewPublisher(brokers []string, topic string) (*Publisher, error) {
	newConfig := sarama.NewConfig()
	newConfig.Producer.RequiredAcks = sarama.WaitForAll
	newConfig.Producer.Retry.Max = 5
	newConfig.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer(brokers, newConfig)
	if err != nil {
		return nil, err
	}

	return &Publisher{
		producer: producer,
		topic:    topic,
	}, nil
}

// DefaultPublisher returns an event publisher. If no Kafka brokers are
// configured, or the brokers are unreachable, it degrades to a no-op publisher
// (logging a warning) rather than crashing the process — event publishing is a
// non-critical side channel, so the API must still start and serve without it.
func DefaultPublisher() *Publisher {
	brokers := nonEmptyBrokers(config.AppConfig.Brokers)
	if len(brokers) == 0 {
		log.Printf("Kafka disabled: no brokers configured; events will not be published")
		return &Publisher{producer: nil, topic: config.AppConfig.Topic}
	}
	producer, err := NewPublisher(brokers, config.AppConfig.Topic)
	if err != nil {
		log.Printf("Kafka unavailable (%v); events will not be published", err)
		return &Publisher{producer: nil, topic: config.AppConfig.Topic}
	}
	return producer
}

func nonEmptyBrokers(brokers []string) []string {
	out := make([]string, 0, len(brokers))
	for _, b := range brokers {
		if strings.TrimSpace(b) != "" {
			out = append(out, b)
		}
	}
	return out
}

func (p *Publisher) Close() error {
	if p.producer == nil {
		return nil
	}
	return p.producer.Close()
}

func (p *Publisher) Publish(event *AutomationEvent) error {
	if p.producer == nil {
		// Kafka disabled/unavailable: publishing is a no-op.
		return nil
	}
	message, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic: p.topic,
		Value: sarama.StringEncoder(message),
		Key:   sarama.StringEncoder(event.Automation.ID.String()),
	}

	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		return err
	}

	log.Printf("Sent message to Kafka topic %s", p.topic)
	return nil
}
