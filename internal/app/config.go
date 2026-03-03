package app

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port                  string
	BaseURL               string
	WebSocketConnectorURL string
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	ShutdownTimeout       time.Duration
	EventInterval         time.Duration
	LogLevel              string
}

func LoadConfig() Config {
	port := getEnv("PORT", "8081")
	baseURL := getEnv("BASE_URL", "")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}

	return Config{
		Port:                  port,
		BaseURL:               strings.TrimRight(baseURL, "/"),
		WebSocketConnectorURL: strings.TrimRight(getEnv("WS_CONNECTOR_BASE_URL", "http://web-socket-connector:8091"), "/"),
		ReadTimeout:           getEnvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:          getEnvDuration("WRITE_TIMEOUT", 20*time.Second),
		ShutdownTimeout:       getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		EventInterval:         getEnvDuration("EVENT_INTERVAL", 5*time.Second),
		LogLevel:              strings.ToLower(getEnv("LOG_LEVEL", "info")),
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
