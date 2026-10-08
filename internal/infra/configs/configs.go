package configs

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/spf13/viper"
)

type Configs struct {
	ApplicationName string        `mapstructure:"application_name"`
	Env             string        `mapstructure:"env"`
	Transaction     Transaction   `mapstructure:"transaction"`
	Servers         Servers       `mapstructure:"servers"`
	DataBase        DataBase      `mapstructure:"data_base"`
	OpenTelemetry   OpemTelemetry `mapstructure:"open_telemetry"`
	Nats            Nats          `mapstructure:"nats"`
	Version         string        `mapstructure:"version"`
}

type Transaction struct {
	MaxEntries int `mapstructure:"max_entries"`
}

type Servers struct {
	GRPC GRPC `mapstructure:"grpc"`
	HTTP HTTP `mapstructure:"http"`
}
type GRPC struct {
	Port string `mapstructure:"port"`
}

type HTTP struct {
	Port string `mapstructure:"port"`
}

type DataBase struct {
	Postgres Postgres `mapstructure:"postgres"`
}

type OpemTelemetry struct {
	Host string `mapstructure:"host"`
}

type Nats struct {
	URL      string       `mapstructure:"url"`
	Name     string       `mapstructure:"name"`
	Subject  string       `mapstructure:"subject"`
	Consumer NatsConsumer `mapstructure:"consumer"`
	Relay    OutboxRelay  `mapstructure:"relay"`
}

type OutboxRelay struct {
	Stream          string        `mapstructure:"stream"`
	Subject         string        `mapstructure:"subject"`
	DLQSubject      string        `mapstructure:"dlq_subject"`
	BatchSize       int           `mapstructure:"batch_size"`
	MaxWorkers      int           `mapstructure:"max_workers"`
	MaxAttempts     int           `mapstructure:"max_attempts"`
	PollInterval    time.Duration `mapstructure:"poll_interval"`
	RetryAfter      time.Duration `mapstructure:"retry_after"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type NatsConsumer struct {
	Stream         string        `mapstructure:"stream"`
	Name           string        `mapstructure:"name"`
	Durable        string        `mapstructure:"durable"`
	AckWait        time.Duration `mapstructure:"ack_wait"`
	MaxDeliver     int           `mapstructure:"max_deliver"`
	MaxWorkers     int           `mapstructure:"max_workers"`
	MaxMessageSize int           `mapstructure:"max_message_size"`
}
type Postgres struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxConnections  int32         `mapstructure:"max_connections"`
	MinConnections  int32         `mapstructure:"min_connections"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time"`
}

func LoadConfig() (*Configs, error) {
	config := viper.New()
	config.SetConfigName("config")
	config.SetConfigType("json")
	config.AddConfigPath(".")
	config.AddConfigPath("/")
	config.SetDefault("transaction.max_entries", application.DefaultMaxTransactionEntries)
	config.AutomaticEnv()
	config.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	bindEnvs(config)

	if err := config.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	var configs Configs
	if err := config.Unmarshal(&configs); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := os.Setenv("ENV", configs.Env); err != nil {
		return nil, fmt.Errorf("set ENV from config: %w", err)
	}
	return &configs, nil
}

// bindEnvs maps environment variables to the keys in config.json.
// Env vars take precedence over the configuration file.
func bindEnvs(config *viper.Viper) {
	_ = config.BindEnv("transaction.max_entries", "TRANSACTION_MAX_ENTRIES")
	_ = config.BindEnv("env", "APP_ENV")
	_ = config.BindEnv("application_name", "APPLICATION_NAME")
	_ = config.BindEnv("Version", "VERSION")
	_ = config.BindEnv("servers.grpc.port", "GRPC_PORT")
	_ = config.BindEnv("servers.http.port", "HTTP_PORT")
	_ = config.BindEnv("data_base.postgres.host", "POSTGRES_HOST")
	_ = config.BindEnv("data_base.postgres.port", "POSTGRES_PORT")
	_ = config.BindEnv("data_base.postgres.user", "POSTGRES_USER")
	_ = config.BindEnv("data_base.postgres.password", "POSTGRES_PASSWORD")
	_ = config.BindEnv("data_base.postgres.database", "POSTGRES_DB")
	_ = config.BindEnv("data_base.postgres.ssl_mode", "POSTGRES_SSL_MODE")
	_ = config.BindEnv("data_base.postgres.max_connections", "POSTGRES_MAX_CONNECTIONS")
	_ = config.BindEnv("data_base.postgres.min_connections", "POSTGRES_MIN_CONNECTIONS")
	_ = config.BindEnv("data_base.postgres.max_conn_lifetime", "POSTGRES_MAX_CONN_LIFETIME")
	_ = config.BindEnv("data_base.postgres.max_conn_idle_time", "POSTGRES_MAX_CONN_IDLE_TIME")
	_ = config.BindEnv("open_telemetry.host", "OTEL_HOST")
	_ = config.BindEnv("nats.url", "NATS_URL")
	_ = config.BindEnv("nats.name", "NATS_NAME")
	_ = config.BindEnv("nats.subject", "NATS_SUBJECT")
	_ = config.BindEnv("nats.consumer.stream", "NATS_CONSUMER_STREAM")
	_ = config.BindEnv("nats.consumer.name", "NATS_CONSUMER_NAME")
	_ = config.BindEnv("nats.consumer.durable", "NATS_CONSUMER_DURABLE")
	_ = config.BindEnv("nats.consumer.ack_wait", "NATS_CONSUMER_ACK_WAIT")
	_ = config.BindEnv("nats.consumer.max_deliver", "NATS_CONSUMER_MAX_DELIVER")
	_ = config.BindEnv("nats.consumer.max_workers", "NATS_CONSUMER_MAX_WORKERS")
	_ = config.BindEnv("nats.consumer.max_message_size", "NATS_CONSUMER_MAX_MESSAGE_SIZE")
	_ = config.BindEnv("nats.relay.stream", "NATS_RELAY_STREAM")
	_ = config.BindEnv("nats.relay.subject", "NATS_RELAY_SUBJECT")
	_ = config.BindEnv("nats.relay.dlq_subject", "NATS_RELAY_DLQ_SUBJECT")
	_ = config.BindEnv("nats.relay.batch_size", "NATS_RELAY_BATCH_SIZE")
	_ = config.BindEnv("nats.relay.max_workers", "NATS_RELAY_MAX_WORKERS")
	_ = config.BindEnv("nats.relay.max_attempts", "NATS_RELAY_MAX_ATTEMPTS")
	_ = config.BindEnv("nats.relay.poll_interval", "NATS_RELAY_POLL_INTERVAL")
	_ = config.BindEnv("nats.relay.retry_after", "NATS_RELAY_RETRY_AFTER")
}
