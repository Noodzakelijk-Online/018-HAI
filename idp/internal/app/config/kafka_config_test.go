package config

import (
	"strings"
	"testing"
)

func TestKafkaConfigTrimsRequiredValues(t *testing.T) {
	t.Setenv(loggerTopic, " idp-logs ")
	t.Setenv(mailTopic, " idp-mail ")
	t.Setenv(brokersAddr, " kafka:9092, ,backup:9092 ")
	t.Setenv(clientID, " hai-idp ")

	cfg, err := newKafkaConfig()
	if err != nil {
		t.Fatalf("newKafkaConfig() error = %v", err)
	}
	if cfg.LoggerTopic != "idp-logs" || cfg.MailTopic != "idp-mail" || cfg.ClientID != "hai-idp" {
		t.Fatalf("unexpected normalized config: %#v", cfg)
	}
	if got := strings.Join(cfg.BrokersAddr, ","); got != "kafka:9092,backup:9092" {
		t.Fatalf("brokers = %q", got)
	}
}

func TestKafkaConfigIsOmittedWhenEventBusIsDisabled(t *testing.T) {
	t.Setenv(kafkaEnabled, "false")

	cfg, err := newOptionalKafkaConfig()
	if err != nil {
		t.Fatalf("newOptionalKafkaConfig() error = %v", err)
	}
	if cfg != nil {
		t.Fatalf("newOptionalKafkaConfig() = %#v, want nil when disabled", cfg)
	}
}

func TestKafkaConfigRejectsMissingRequiredValues(t *testing.T) {
	for _, test := range []struct {
		name string
		key  string
		want string
	}{
		{name: "logger topic", key: loggerTopic, want: loggerTopic},
		{name: "mail topic", key: mailTopic, want: mailTopic},
		{name: "brokers", key: brokersAddr, want: brokersAddr},
		{name: "client id", key: clientID, want: clientID},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(loggerTopic, "logs")
			t.Setenv(mailTopic, "mail")
			t.Setenv(brokersAddr, "kafka:9092")
			t.Setenv(clientID, "hai-idp")
			t.Setenv(test.key, " ")

			_, err := newKafkaConfig()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("newKafkaConfig() error = %v, want context for %s", err, test.want)
			}
		})
	}
}
