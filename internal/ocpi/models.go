package ocpi

import "time"

type Response[T any] struct {
	StatusCode    int       `json:"status_code"`
	StatusMessage string    `json:"status_message,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Data          T         `json:"data"`
}

type EmptyResponse struct {
	StatusCode    int       `json:"status_code"`
	StatusMessage string    `json:"status_message,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

type Version struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type Endpoint struct {
	Identifier string `json:"identifier"`
	URL        string `json:"url"`
}

type Credentials struct {
	Token      string    `json:"token"`
	URL        string    `json:"url"`
	PartyID    string    `json:"party_id"`
	CountryCode string   `json:"country_code"`
	Roles      []Role    `json:"roles"`
	BusinessDetails BusinessDetails `json:"business_details"`
	LastUpdated time.Time `json:"last_updated"`
}

type Role struct {
	Role          string `json:"role"`
	PartyID       string `json:"party_id"`
	CountryCode   string `json:"country_code"`
	BusinessDetails BusinessDetails `json:"business_details"`
}

type BusinessDetails struct {
	Name string `json:"name"`
}

type Coordinates struct {
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
}

type Location struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Address     string     `json:"address"`
	City        string     `json:"city"`
	Country     string     `json:"country"`
	Coordinates Coordinates `json:"coordinates"`
	Evse        []EVSE     `json:"evses"`
	LastUpdated time.Time  `json:"last_updated"`
}

type EVSE struct {
	UID        string      `json:"uid"`
	Status     string      `json:"status"`
	Connectors []Connector `json:"connectors"`
	LastUpdated time.Time  `json:"last_updated"`
}

type Connector struct {
	ID          string `json:"id"`
	Standard    string `json:"standard"`
	Format      string `json:"format"`
	PowerType   string `json:"power_type"`
	MaxVoltage  int    `json:"max_voltage"`
	MaxAmperage int    `json:"max_amperage"`
}

type Tariff struct {
	ID        string    `json:"id"`
	Currency  string    `json:"currency"`
	LastUpdated time.Time `json:"last_updated"`
}

type Session struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Kwh        float64   `json:"kwh"`
	LocationID string    `json:"location_id"`
	EvseUID    string    `json:"evse_uid"`
	StartDateTime time.Time `json:"start_datetime"`
	EndDateTime   *time.Time `json:"end_datetime,omitempty"`
	LastUpdated   time.Time `json:"last_updated"`
}

type CDR struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	TotalCost  float64   `json:"total_cost"`
	TotalEnergy float64  `json:"total_energy"`
	LastUpdated time.Time `json:"last_updated"`
}

type CommandResponse struct {
	Result  string `json:"result"`
	Timeout int    `json:"timeout"`
	Message string `json:"message,omitempty"`
}
