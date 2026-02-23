package fleet

import "time"

type Transport struct {
	Role    string `json:"role"`
	CSMSURL string `json:"csmsUrl"`
	TLS     TLS    `json:"tls"`
}

type TLS struct {
	Enabled    bool `json:"enabled"`
	SkipVerify bool `json:"skipVerify"`
}

type Connector struct {
	ConnectorID int     `json:"connectorId"`
	Type        string  `json:"type"`
	MaxKw       float64 `json:"maxKw"`
	Status      string  `json:"status,omitempty"`
	ErrorCode   string  `json:"errorCode,omitempty"`
	FaultType   string  `json:"faultType,omitempty"`
}

type SOCConfig struct {
	Enabled           bool    `json:"enabled"`
	StartPercent      int     `json:"startPercent"`
	EndPercent        int     `json:"endPercent"`
	RatePercentPerMin float64 `json:"ratePercentPerMin"`
}

type MeteringConfig struct {
	EnergyWhStart int64 `json:"energyWhStart"`
	PowerW        int64 `json:"powerW"`
	VoltageV      int64 `json:"voltageV"`
	CurrentA      int64 `json:"currentA"`
}

type ClockConfig struct {
	TimeZone      string `json:"timeZone"`
	DriftMsPerMin int64  `json:"driftMsPerMin"`
}

type BootConfig struct {
	Vendor          string `json:"vendor"`
	Model           string `json:"model"`
	FirmwareVersion string `json:"firmwareVersion"`
}

type ChargerConfig struct {
	HeartbeatIntervalSec   int            `json:"heartbeatIntervalSec"`
	MeterValuesIntervalSec int            `json:"meterValuesIntervalSec"`
	SOC                    SOCConfig      `json:"soc"`
	Metering               MeteringConfig `json:"metering"`
	Clock                  ClockConfig    `json:"clock"`
	Boot                   BootConfig     `json:"boot"`
}

type Transaction struct {
	TransactionID  string     `json:"transactionId"`
	ConnectorID    int        `json:"connectorId"`
	Status         string     `json:"status"`
	MeterStartWh   int64      `json:"meterStartWh"`
	MeterStopWh    int64      `json:"meterStopWh"`
	StartedAt      time.Time  `json:"startedAt"`
	StoppedAt      *time.Time `json:"stoppedAt,omitempty"`
	AuthorizationID string    `json:"authorizationId,omitempty"`
	IdTag          string     `json:"idTag,omitempty"`
	IdToken        string     `json:"idToken,omitempty"`
}

type Runtime struct {
	LastHeartbeatAt   *time.Time   `json:"lastHeartbeatAt,omitempty"`
	LastMessageAt     *time.Time   `json:"lastMessageAt,omitempty"`
	ActiveTransactions []Transaction `json:"activeTransactions"`
}

type Charger struct {
	ChargerID       string            `json:"chargerId"`
	OCPPIdentity    string            `json:"ocppIdentity"`
	OCPPVersion     string            `json:"ocppVersion"`
	Transport       Transport         `json:"transport"`
	Connectors      []Connector       `json:"connectors"`
	Config          ChargerConfig     `json:"config"`
	Tags            map[string]string `json:"tags"`
	ConnectionState string            `json:"connectionState"`
	ConnectionRemote string           `json:"connectionRemote,omitempty"`
	ConnectionSince  *time.Time       `json:"connectionSince,omitempty"`
	Runtime         Runtime           `json:"runtime"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

type ReconnectPolicy struct {
	Enabled      bool `json:"enabled"`
	MinBackoffMs int  `json:"minBackoffMs"`
	MaxBackoffMs int  `json:"maxBackoffMs"`
}

type Event struct {
	Type          string         `json:"type"`
	Timestamp     time.Time      `json:"timestamp"`
	ChargerID     string         `json:"chargerId,omitempty"`
	ConnectorID   int            `json:"connectorId,omitempty"`
	TransactionID string         `json:"transactionId,omitempty"`
	Result        string         `json:"result,omitempty"`
	Message       string         `json:"message,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
}

type BulkJob struct {
	JobID     string    `json:"jobId"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Total     int       `json:"total"`
	Completed int       `json:"completed"`
}
