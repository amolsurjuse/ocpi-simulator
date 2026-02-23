package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Charger struct {
	ID          string `json:"id"`
	LocationID  string `json:"location_id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	City        string `json:"city"`
	State       string `json:"state"`
	PostalCode  string `json:"postal_code"`
	Country     string `json:"country"`
	Latitude    string `json:"latitude"`
	Longitude   string `json:"longitude"`
	EvseUID     string `json:"evse_uid"`
	ConnectorID string `json:"connector_id"`
	Status      string `json:"status"`
	MaxVoltage  int    `json:"max_voltage"`
	MaxAmperage int    `json:"max_amperage"`
	OCPPProtocol string `json:"ocpp_protocol"`
	ChargePointID string `json:"charge_point_id"`
}

type Session struct {
	ID         string    `json:"id"`
	ChargerID  string    `json:"charger_id"`
	LocationID string    `json:"location_id"`
	EvseUID    string    `json:"evse_uid"`
	Status     string    `json:"status"`
	Kwh        float64   `json:"kwh"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at,omitempty"`
}

type Tariff struct {
	ID       string `json:"id"`
	Currency string `json:"currency"`
	Price    string `json:"price"`
}

type Store struct {
	mu       sync.RWMutex
	chargers map[string]Charger
	sessions map[string]Session
	tariffs  map[string]Tariff
}

func NewStore() *Store {
	defaultTariff := Tariff{
		ID:       "tariff-basic",
		Currency: "USD",
		Price:    "0.35",
	}

	return &Store{
		chargers: map[string]Charger{},
		sessions: map[string]Session{},
		tariffs:  map[string]Tariff{defaultTariff.ID: defaultTariff},
	}
}

func (s *Store) AddCharger(input Charger) Charger {
	s.mu.Lock()
	defer s.mu.Unlock()

	charger := normalizeCharger(input)
	s.chargers[charger.ID] = charger
	return charger
}

func (s *Store) UpdateCharger(id string, input Charger) (Charger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.chargers[id]
	if !ok {
		return Charger{}, errors.New("charger not found")
	}

	input.ID = id
	input.LocationID = current.LocationID
	charger := normalizeCharger(input)
	s.chargers[id] = charger
	return charger, nil
}

func (s *Store) RemoveCharger(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.chargers[id]; !ok {
		return errors.New("charger not found")
	}
	delete(s.chargers, id)
	return nil
}

func (s *Store) GetCharger(id string) (Charger, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	charger, ok := s.chargers[id]
	return charger, ok
}

func (s *Store) ListChargers() []Charger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Charger, 0, len(s.chargers))
	for _, charger := range s.chargers {
		items = append(items, charger)
	}
	return items
}

func (s *Store) FindChargerByChargePointID(chargePointID string) (Charger, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, charger := range s.chargers {
		if charger.ChargePointID == chargePointID {
			return charger, true
		}
	}
	return Charger{}, false
}

func (s *Store) AddSession(input Session) Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := input
	if session.ID == "" {
		session.ID = newID("ses")
	}
	if session.Status == "" {
		session.Status = "ACTIVE"
	}
	if session.StartedAt.IsZero() {
		session.StartedAt = time.Now().UTC()
	}
	s.sessions[session.ID] = session
	return session
}

func (s *Store) UpdateSession(session Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session
}

func (s *Store) GetSession(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	return session, ok
}

func (s *Store) ListSessions() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, session)
	}
	return items
}

func (s *Store) StopSession(id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, errors.New("session not found")
	}
	session.Status = "COMPLETED"
	session.EndedAt = time.Now().UTC()
	s.sessions[id] = session
	return session, nil
}

func (s *Store) ListTariffs() []Tariff {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Tariff, 0, len(s.tariffs))
	for _, tariff := range s.tariffs {
		items = append(items, tariff)
	}
	return items
}

func normalizeCharger(input Charger) Charger {
	charger := input
	if charger.ID == "" {
		charger.ID = newID("chg")
	}
	if charger.LocationID == "" {
		charger.LocationID = "loc-" + charger.ID
	}
	if charger.Name == "" {
		charger.Name = "Electra Hub Charger"
	}
	if charger.Country == "" {
		charger.Country = "US"
	}
	if charger.Latitude == "" {
		charger.Latitude = "37.7749"
	}
	if charger.Longitude == "" {
		charger.Longitude = "-122.4194"
	}
	if charger.EvseUID == "" {
		charger.EvseUID = "EVSE-" + charger.ID
	}
	if charger.ConnectorID == "" {
		charger.ConnectorID = "1"
	}
	if charger.Status == "" {
		charger.Status = "AVAILABLE"
	}
	if charger.OCPPProtocol == "" {
		charger.OCPPProtocol = "1.6"
	}
	if charger.ChargePointID == "" {
		charger.ChargePointID = "CP-" + charger.ID
	}
	if charger.MaxVoltage == 0 {
		charger.MaxVoltage = 400
	}
	if charger.MaxAmperage == 0 {
		charger.MaxAmperage = 32
	}
	return charger
}

func newID(prefix string) string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405")
	}
	return prefix + "-" + hex.EncodeToString(buf)
}
