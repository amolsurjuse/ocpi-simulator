package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"ocpi-simulator/internal/store"
)

type meterRequest struct {
	Kwh float64 `json:"kwh"`
}

type chargerRequest struct {
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

type sessionRequest struct {
	ChargerID  string `json:"charger_id"`
	LocationID string `json:"location_id"`
	EvseUID    string `json:"evse_uid"`
}

func (a *App) handleListChargers(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, a.store.ListChargers())
}

func (a *App) handleCreateCharger(w http.ResponseWriter, r *http.Request) {
	var payload chargerRequest
	if err := decodeJSON(w, r, &payload); err != nil {
		return
	}

	charger := a.store.AddCharger(store.Charger{
		ID:          payload.ID,
		LocationID:  payload.LocationID,
		Name:        payload.Name,
		Address:     payload.Address,
		City:        payload.City,
		State:       payload.State,
		PostalCode:  payload.PostalCode,
		Country:     payload.Country,
		Latitude:    payload.Latitude,
		Longitude:   payload.Longitude,
		EvseUID:     payload.EvseUID,
		ConnectorID: payload.ConnectorID,
		Status:      payload.Status,
		MaxVoltage:  payload.MaxVoltage,
		MaxAmperage: payload.MaxAmperage,
		OCPPProtocol: payload.OCPPProtocol,
		ChargePointID: payload.ChargePointID,
	})

	a.emitEvent(Event{
		Type:       "dms",
		Timestamp:  time.Now().UTC(),
		ChargerID:  charger.ID,
		LocationID: charger.LocationID,
		EvseUID:    charger.EvseUID,
		Message:    "charger_registered",
	})

	respondJSON(w, http.StatusCreated, charger)
}

func (a *App) handleGetCharger(w http.ResponseWriter, r *http.Request, chargerID string) {
	charger, ok := a.store.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	respondJSON(w, http.StatusOK, charger)
}

func (a *App) handleDeleteCharger(w http.ResponseWriter, r *http.Request, chargerID string) {
	if err := a.store.RemoveCharger(chargerID); err != nil {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleStartSession(w http.ResponseWriter, r *http.Request, chargerID string) {
	charger, ok := a.store.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var payload sessionRequest
	_ = decodeJSON(w, r, &payload)

	session := a.store.AddSession(store.Session{
		ChargerID:  charger.ID,
		LocationID: charger.LocationID,
		EvseUID:    charger.EvseUID,
		Status:     "ACTIVE",
	})

	a.emitEvent(Event{
		Type:       "start",
		Timestamp:  time.Now().UTC(),
		ChargerID:  charger.ID,
		LocationID: charger.LocationID,
		EvseUID:    charger.EvseUID,
		SessionID:  session.ID,
		Message:    "session_started",
	})

	respondJSON(w, http.StatusCreated, session)
}

func (a *App) handleStopSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	session, err := a.store.StopSession(sessionID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	a.emitEvent(Event{
		Type:       "stop",
		Timestamp:  time.Now().UTC(),
		ChargerID:  session.ChargerID,
		LocationID: session.LocationID,
		EvseUID:    session.EvseUID,
		SessionID:  session.ID,
		Kwh:        session.Kwh,
		Message:    "session_stopped",
	})

	respondJSON(w, http.StatusOK, session)
}

func (a *App) handleMeterValue(w http.ResponseWriter, r *http.Request, sessionID string) {
	session, ok := a.store.GetSession(sessionID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var payload meterRequest
	if err := decodeJSON(w, r, &payload); err != nil {
		return
	}

	if payload.Kwh > 0 {
		session.Kwh += payload.Kwh
		a.store.UpdateSession(session)
	}

	a.emitEvent(Event{
		Type:       "meter_value",
		Timestamp:  time.Now().UTC(),
		ChargerID:  session.ChargerID,
		LocationID: session.LocationID,
		EvseUID:    session.EvseUID,
		SessionID:  session.ID,
		Kwh:        session.Kwh,
		MeterValue: session.Kwh,
	})

	respondJSON(w, http.StatusOK, session)
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		http.Error(w, "invalid json", http.StatusBadRequest)
		return err
	}
	return nil
}
