package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment string
	LogLevel    string
	HTTP        HTTPConfig
	Database    DatabaseConfig
	SQS         SQSConfig
	OIDC        OIDCConfig
}

type HTTPConfig struct {
	Address          string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	ReadinessTimeout time.Duration
	ShutdownTimeout  time.Duration
}

type DatabaseConfig struct {
	URL            string
	ConnectTimeout time.Duration
	MaxConnections int32
	MinConnections int32
}

type SQSConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	InputQueue      string
	DLQ             string
	EventsQueue     string
}

type OIDCConfig struct {
	Issuer   string
	Audience string
}

func Load() (Config, error) {
	readTimeout, err := durationFromEnv("HTTP_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationFromEnv("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationFromEnv("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	readinessTimeout, err := durationFromEnv("READINESS_TIMEOUT", 2*time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromEnv("SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	databaseConnectTimeout, err := durationFromEnv("DATABASE_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	maxConnections, err := int32FromEnv("DATABASE_MAX_CONNECTIONS", 20)
	if err != nil {
		return Config{}, err
	}
	minConnections, err := int32FromEnv("DATABASE_MIN_CONNECTIONS", 2)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Environment: valueFromEnv("APP_ENV", "local"),
		LogLevel:    valueFromEnv("LOG_LEVEL", "info"),
		HTTP: HTTPConfig{
			Address:          valueFromEnv("HTTP_ADDRESS", ":8080"),
			ReadTimeout:      readTimeout,
			WriteTimeout:     writeTimeout,
			IdleTimeout:      idleTimeout,
			ReadinessTimeout: readinessTimeout,
			ShutdownTimeout:  shutdownTimeout,
		},
		Database: DatabaseConfig{
			URL:            valueFromEnv("DATABASE_URL", "postgres://jungle_app:jungle-local-password@localhost:5432/jungle_gaming?sslmode=disable"),
			ConnectTimeout: databaseConnectTimeout,
			MaxConnections: maxConnections,
			MinConnections: minConnections,
		},
		SQS: SQSConfig{
			Endpoint:        valueFromEnv("SQS_ENDPOINT", "http://localhost:4566"),
			Region:          valueFromEnv("AWS_REGION", "us-east-1"),
			AccessKeyID:     valueFromEnv("AWS_ACCESS_KEY_ID", "jungle-local"),
			SecretAccessKey: valueFromEnv("AWS_SECRET_ACCESS_KEY", "jungle-local"),
			InputQueue:      valueFromEnv("SQS_INPUT_QUEUE", "wager-transactions.fifo"),
			DLQ:             valueFromEnv("SQS_DLQ", "wager-transactions-dlq.fifo"),
			EventsQueue:     valueFromEnv("SQS_EVENTS_QUEUE", "wager-events.fifo"),
		},
		OIDC: OIDCConfig{
			Issuer:   valueFromEnv("OIDC_ISSUER", "http://localhost:8081/realms/jungle-gaming"),
			Audience: valueFromEnv("OIDC_AUDIENCE", "jungle-api"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate configuration: %w", err)
	}

	return cfg, nil
}

func (c Config) Validate() error {
	var errs []error

	if _, _, err := net.SplitHostPort(c.HTTP.Address); err != nil {
		errs = append(errs, fmt.Errorf("HTTP_ADDRESS must be host:port: %w", err))
	}
	if c.HTTP.ReadTimeout <= 0 || c.HTTP.WriteTimeout <= 0 || c.HTTP.IdleTimeout <= 0 {
		errs = append(errs, errors.New("HTTP timeouts must be positive"))
	}
	if c.HTTP.ReadinessTimeout <= 0 || c.HTTP.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("readiness and shutdown timeouts must be positive"))
	}
	if c.Database.ConnectTimeout <= 0 {
		errs = append(errs, errors.New("DATABASE_CONNECT_TIMEOUT must be positive"))
	}
	if c.Database.MaxConnections <= 0 {
		errs = append(errs, errors.New("DATABASE_MAX_CONNECTIONS must be positive"))
	}
	if c.Database.MinConnections < 0 || c.Database.MinConnections > c.Database.MaxConnections {
		errs = append(errs, errors.New("DATABASE_MIN_CONNECTIONS must be between zero and DATABASE_MAX_CONNECTIONS"))
	}
	if err := validateURL("DATABASE_URL", c.Database.URL, "postgres", "postgresql"); err != nil {
		errs = append(errs, err)
	}
	if err := validateURL("SQS_ENDPOINT", c.SQS.Endpoint, "http", "https"); err != nil {
		errs = append(errs, err)
	}
	if err := validateURL("OIDC_ISSUER", c.OIDC.Issuer, "http", "https"); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(c.SQS.Region) == "" || strings.TrimSpace(c.SQS.AccessKeyID) == "" || strings.TrimSpace(c.SQS.SecretAccessKey) == "" {
		errs = append(errs, errors.New("AWS region and SQS credentials must not be empty"))
	}
	if strings.TrimSpace(c.SQS.InputQueue) == "" || strings.TrimSpace(c.SQS.DLQ) == "" || strings.TrimSpace(c.SQS.EventsQueue) == "" {
		errs = append(errs, errors.New("SQS queue names must not be empty"))
	}
	if strings.TrimSpace(c.OIDC.Audience) == "" {
		errs = append(errs, errors.New("OIDC_AUDIENCE must not be empty"))
	}

	return errors.Join(errs...)
}

func validateURL(name, raw string, schemes ...string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}
	for _, scheme := range schemes {
		if parsed.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported scheme %q", name, parsed.Scheme)
}

func valueFromEnv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := valueFromEnv(name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return value, nil
}

func int32FromEnv(name string, fallback int32) (int32, error) {
	raw := valueFromEnv(name, strconv.FormatInt(int64(fallback), 10))
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return int32(value), nil
}
