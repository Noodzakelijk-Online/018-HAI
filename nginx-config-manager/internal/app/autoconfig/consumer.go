package autoconfig

import (
	"automation-hub-nginxconfigmanager/internal/app/config"
	"fmt"
	"github.com/IBM/sarama"
	"log"
	"sync/atomic"
)

type Consumer struct {
	consumer sarama.Consumer
	topic    string
	ready    atomic.Bool
}

func NewConsumer(brokers []string, topic string) (*Consumer, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Consumer.Return.Errors = true
	consumer, err := sarama.NewConsumer(brokers, saramaConfig)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		consumer: consumer,
		topic:    topic,
	}, nil
}

func DefaultConsumer() (*Consumer, error) {
	consumer, err := NewConsumer(config.AppConfig.Brokers, config.AppConfig.Topic)
	if err != nil {
		return nil, fmt.Errorf("create default consumer: %w", err)
	}
	return consumer, nil
}

func (c *Consumer) Ready() bool {
	return c.ready.Load()
}

func (c *Consumer) Start() error {
	partitionConsumer, err := c.consumer.ConsumePartition(c.topic, 0, sarama.OffsetNewest)
	if err != nil {
		return fmt.Errorf("start partition consumer: %w", err)
	}
	defer partitionConsumer.Close()

	messages := partitionConsumer.Messages()
	consumerErrors := partitionConsumer.Errors()
	c.ready.Store(true)
	defer c.ready.Store(false)

	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return fmt.Errorf("Kafka message channel closed")
			}
			if msg == nil {
				continue
			}
			processMessage(msg)
		case consumerErr, ok := <-consumerErrors:
			if !ok {
				consumerErrors = nil
				continue
			}
			if consumerErr != nil {
				log.Printf("Kafka consumer error: %v", consumerErr)
			}
		}
	}
}

func (c *Consumer) Close() error {
	return c.consumer.Close()
}
