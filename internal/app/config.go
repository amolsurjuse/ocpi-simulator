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
	DefaultCSMSURL        string
	AutoSeedChargers      bool
	AutoConnectChargers   bool
	AutoConnectInterval   time.Duration
	AutoChargerIDs        []string
	AutonomousEventCycle  bool
	UIStaticDir           string
	UIEnabled             bool
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	ShutdownTimeout       time.Duration
	EnvironmentTimeout    time.Duration
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
		DefaultCSMSURL:        strings.TrimRight(getEnv("DEFAULT_CSMS_URL", "ws://ocpp-service:8082/ws/ocpp"), "/"),
		AutoSeedChargers:      getEnvBool("AUTO_SEED_CHARGERS", true),
		AutoConnectChargers:   getEnvBool("AUTO_CONNECT_CHARGERS", true),
		AutoConnectInterval:   getEnvDuration("AUTO_CONNECT_INTERVAL", 15*time.Second),
		AutoChargerIDs:        getEnvList("AUTO_CHARGER_IDS", []string{"EH-SFO-CHG-001"}),
		AutonomousEventCycle:  getEnvBool("AUTONOMOUS_EVENT_CYCLE", false),
		UIStaticDir:           strings.TrimSpace(getEnv("UI_STATIC_DIR", "")),
		UIEnabled:             getEnvBool("UI_ENABLED", true),
		ReadTimeout:           getEnvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:          getEnvDuration("WRITE_TIMEOUT", 20*time.Second),
		ShutdownTimeout:       getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		EnvironmentTimeout:    getEnvDuration("ENVIRONMENT_TIMEOUT", 15*time.Second),
		EventInterval:         getEnvDuration("EVENT_INTERVAL", 5*time.Second),
		LogLevel:              strings.ToLower(getEnv("LOG_LEVEL", "info")),
	}
}

func getEnvList(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return fallback
	}
	return items
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

func getEnvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
