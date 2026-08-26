package configs

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Configs struct {
	ApplicationName string        `mapstructure:"application_name"`
	Env             string        `mapstructure:"env"`
	Servers         Servers       `mapstructure:"servers"`
	DataBase        DataBase      `mapstructure:"data_base"`
	OpenTelemetry   OpemTelemetry `mapstructure:"open_telemetry"`
	Nats            Nats          `mapstructure:"nats"`
	Version         string        `mapstructure:"version"`
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

func LoadConfig() *Configs {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return nil
		}
	}

	bindEnvs()

	var configs Configs
	if err := viper.Unmarshal(&configs); err != nil {
		return nil
	}

	_ = os.Setenv("ENV", configs.Env)
	return &configs
}

// bindEnvs maps environment variables to the keys in config.json.
// Env vars take precedence over the configuration file.
func bindEnvs() {
	_ = viper.BindEnv("env", "APP_ENV")
	_ = viper.BindEnv("application_name", "APPLICATION_NAME")
	_ = viper.BindEnv("Version", "VERSION")
	_ = viper.BindEnv("servers.grpc.port", "GRPC_PORT")
	_ = viper.BindEnv("servers.http.port", "HTTP_PORT")
	_ = viper.BindEnv("data_base.postgres.host", "POSTGRES_HOST")
	_ = viper.BindEnv("data_base.postgres.port", "POSTGRES_PORT")
	_ = viper.BindEnv("data_base.postgres.user", "POSTGRES_USER")
	_ = viper.BindEnv("data_base.postgres.password", "POSTGRES_PASSWORD")
	_ = viper.BindEnv("data_base.postgres.database", "POSTGRES_DB")
	_ = viper.BindEnv("data_base.postgres.ssl_mode", "POSTGRES_SSL_MODE")
	_ = viper.BindEnv("data_base.postgres.max_connections", "POSTGRES_MAX_CONNECTIONS")
	_ = viper.BindEnv("data_base.postgres.min_connections", "POSTGRES_MIN_CONNECTIONS")
	_ = viper.BindEnv("data_base.postgres.max_conn_lifetime", "POSTGRES_MAX_CONN_LIFETIME")
	_ = viper.BindEnv("data_base.postgres.max_conn_idle_time", "POSTGRES_MAX_CONN_IDLE_TIME")
	_ = viper.BindEnv("open_telemetry.host", "OTEL_HOST")
	_ = viper.BindEnv("nats.url", "NATS_URL")
	_ = viper.BindEnv("nats.name", "NATS_NAME")
	_ = viper.BindEnv("nats.subject", "NATS_SUBJECT")
	_ = viper.BindEnv("nats.consumer.stream", "NATS_CONSUMER_STREAM")
	_ = viper.BindEnv("nats.consumer.name", "NATS_CONSUMER_NAME")
	_ = viper.BindEnv("nats.consumer.durable", "NATS_CONSUMER_DURABLE")
	_ = viper.BindEnv("nats.consumer.ack_wait", "NATS_CONSUMER_ACK_WAIT")
	_ = viper.BindEnv("nats.consumer.max_deliver", "NATS_CONSUMER_MAX_DELIVER")
	_ = viper.BindEnv("nats.consumer.max_workers", "NATS_CONSUMER_MAX_WORKERS")
	_ = viper.BindEnv("nats.consumer.max_message_size", "NATS_CONSUMER_MAX_MESSAGE_SIZE")
}
