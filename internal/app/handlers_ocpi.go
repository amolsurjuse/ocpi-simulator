package app

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"ocpi-simulator/internal/ocpi"
	"ocpi-simulator/internal/store"
)

func (a *App) handleVersions(w http.ResponseWriter, r *http.Request) {
	versions := []ocpi.Version{
		{Version: "2.2.1", URL: a.cfg.BaseURL + "/ocpi/2.2.1"},
	}
	writeOCPI(w, http.StatusOK, ocpiResponse(versions))
}

func (a *App) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	base := a.cfg.BaseURL + "/ocpi/2.2.1"
	endpoints := []ocpi.Endpoint{
		{Identifier: "credentials", URL: base + "/credentials"},
		{Identifier: "locations", URL: base + "/locations"},
		{Identifier: "tariffs", URL: base + "/tariffs"},
		{Identifier: "sessions", URL: base + "/sessions"},
		{Identifier: "cdrs", URL: base + "/cdrs"},
		{Identifier: "commands", URL: base + "/commands"},
	}
	writeOCPI(w, http.StatusOK, ocpiResponse(endpoints))
}

func (a *App) handleGetCredentials(w http.ResponseWriter, r *http.Request) {
	a.credMu.RLock()
	credentials := a.credentials
	a.credMu.RUnlock()
	writeOCPI(w, http.StatusOK, ocpiResponse(credentials))
}

func (a *App) handlePostCredentials(w http.ResponseWriter, r *http.Request) {
	var payload ocpi.Credentials
	if err := decodeOCPI(w, r, &payload); err != nil {
		return
	}
	payload.LastUpdated = time.Now().UTC()

	a.credMu.Lock()
	a.credentials = payload
	a.credMu.Unlock()

	writeOCPI(w, http.StatusOK, ocpiResponse(payload))
}

func (a *App) handlePutCredentials(w http.ResponseWriter, r *http.Request) {
	var payload ocpi.Credentials
	if err := decodeOCPI(w, r, &payload); err != nil {
		return
	}
	payload.LastUpdated = time.Now().UTC()

	a.credMu.Lock()
	a.credentials = payload
	a.credMu.Unlock()

	writeOCPI(w, http.StatusOK, ocpiResponse(payload))
}

func (a *App) handleLocations(w http.ResponseWriter, r *http.Request) {
	locations := chargersToLocations(a.store.ListChargers())
	writeOCPI(w, http.StatusOK, ocpiResponse(locations))
}

func (a *App) handleLocation(w http.ResponseWriter, r *http.Request, locationID string) {
	locations := chargersToLocations(a.store.ListChargers())
	for _, location := range locations {
		if location.ID == locationID {
			writeOCPI(w, http.StatusOK, ocpiResponse(location))
			return
		}
	}
	writeOCPI(w, http.StatusNotFound, ocpiError("location not found"))
}

func (a *App) handleEvse(w http.ResponseWriter, r *http.Request, locationID, evseUID string) {
	locations := chargersToLocations(a.store.ListChargers())
	for _, location := range locations {
		if location.ID != locationID {
			continue
		}
		for _, evse := range location.Evse {
			if evse.UID == evseUID {
				writeOCPI(w, http.StatusOK, ocpiResponse(evse))
				return
			}
		}
	}
	writeOCPI(w, http.StatusNotFound, ocpiError("evse not found"))
}

func (a *App) handleConnector(w http.ResponseWriter, r *http.Request, locationID, evseUID, connectorID string) {
	locations := chargersToLocations(a.store.ListChargers())
	for _, location := range locations {
		if location.ID != locationID {
			continue
		}
		for _, evse := range location.Evse {
			if evse.UID != evseUID {
				continue
			}
			for _, connector := range evse.Connectors {
				if connector.ID == connectorID {
					writeOCPI(w, http.StatusOK, ocpiResponse(connector))
					return
				}
			}
		}
	}
	writeOCPI(w, http.StatusNotFound, ocpiError("connector not found"))
}

func (a *App) handleTariffs(w http.ResponseWriter, r *http.Request) {
	tariffs := a.store.ListTariffs()
	items := make([]ocpi.Tariff, 0, len(tariffs))
	for _, tariff := range tariffs {
		items = append(items, ocpi.Tariff{
			ID:         tariff.ID,
			Currency:   tariff.Currency,
			LastUpdated: time.Now().UTC(),
		})
	}
	writeOCPI(w, http.StatusOK, ocpiResponse(items))
}

func (a *App) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions := a.store.ListSessions()
	items := make([]ocpi.Session, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, mapSession(session))
	}
	writeOCPI(w, http.StatusOK, ocpiResponse(items))
}

func (a *App) handleSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	session, ok := a.store.GetSession(sessionID)
	if !ok {
		writeOCPI(w, http.StatusNotFound, ocpiError("session not found"))
		return
	}
	writeOCPI(w, http.StatusOK, ocpiResponse(mapSession(session)))
}

func (a *App) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var payload ocpi.Session
	if err := decodeOCPI(w, r, &payload); err != nil {
		return
	}

	chargerID := ""
	if payload.EvseUID != "" {
		chargers := a.store.ListChargers()
		for _, charger := range chargers {
			if charger.EvseUID == payload.EvseUID {
				chargerID = charger.ID
				payload.LocationID = charger.LocationID
				break
			}
		}
	}

	session := a.store.AddSession(store.Session{
		ID:         payload.ID,
		ChargerID:  chargerID,
		LocationID: payload.LocationID,
		EvseUID:    payload.EvseUID,
		Status:     "ACTIVE",
	})

	a.emitEvent(Event{
		Type:       "start",
		Timestamp:  time.Now().UTC(),
		ChargerID:  session.ChargerID,
		LocationID: session.LocationID,
		EvseUID:    session.EvseUID,
		SessionID:  session.ID,
		Message:    "session_started",
	})

	writeOCPI(w, http.StatusOK, ocpiResponse(mapSession(session)))
}

func (a *App) handlePatchSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	session, ok := a.store.GetSession(sessionID)
	if !ok {
		writeOCPI(w, http.StatusNotFound, ocpiError("session not found"))
		return
	}

	var payload ocpi.Session
	if err := decodeOCPI(w, r, &payload); err != nil {
		return
	}

	if payload.Status != "" {
		session.Status = payload.Status
	}
	if payload.Kwh > 0 {
		session.Kwh = payload.Kwh
	}
	if payload.EndDateTime != nil {
		session.EndedAt = *payload.EndDateTime
	}
	if session.Status == "COMPLETED" && session.EndedAt.IsZero() {
		session.EndedAt = time.Now().UTC()
	}
	if session.Status == "COMPLETED" {
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
	}

	a.store.UpdateSession(session)
	writeOCPI(w, http.StatusOK, ocpiResponse(mapSession(session)))
}

func (a *App) handleCDRs(w http.ResponseWriter, r *http.Request) {
	cdrs := []ocpi.CDR{}
	writeOCPI(w, http.StatusOK, ocpiResponse(cdrs))
}

func (a *App) handleCreateCDR(w http.ResponseWriter, r *http.Request) {
	var payload ocpi.CDR
	if err := decodeOCPI(w, r, &payload); err != nil {
		return
	}
	if payload.LastUpdated.IsZero() {
		payload.LastUpdated = time.Now().UTC()
	}
	writeOCPI(w, http.StatusOK, ocpiResponse(payload))
}

func (a *App) handleCommand(w http.ResponseWriter, r *http.Request, command string) {
	command = normalizeCommand(command)
	response := ocpi.CommandResponse{Result: "ACCEPTED", Timeout: 30}

	type commandPayload struct {
		SessionID string `json:"session_id"`
		ChargerID string `json:"charger_id"`
		EvseUID   string `json:"evse_uid"`
	}

	var payload commandPayload
	_ = decodeOCPI(w, r, &payload)

	switch command {
	case "START_SESSION":
		session := a.store.AddSession(store.Session{
			ChargerID: payload.ChargerID,
			EvseUID:   payload.EvseUID,
			Status:    "ACTIVE",
		})
		a.emitEvent(Event{
			Type:       "start",
			Timestamp:  time.Now().UTC(),
			ChargerID:  session.ChargerID,
			EvseUID:    session.EvseUID,
			SessionID:  session.ID,
			Message:    "command_start_session",
		})
	case "STOP_SESSION":
		sessionID := payload.SessionID
		if sessionID == "" {
			for _, session := range a.store.ListSessions() {
				if session.Status == "ACTIVE" {
					sessionID = session.ID
					break
				}
			}
		}
		if sessionID == "" {
			response.Result = "REJECTED"
			response.Message = "no active session found"
			break
		}
		session, err := a.store.StopSession(sessionID)
		if err != nil {
			response.Result = "REJECTED"
			response.Message = "session not found"
			break
		}
		a.emitEvent(Event{
			Type:       "stop",
			Timestamp:  time.Now().UTC(),
			ChargerID:  session.ChargerID,
			LocationID: session.LocationID,
			EvseUID:    session.EvseUID,
			SessionID:  session.ID,
			Kwh:        session.Kwh,
			Message:    "command_stop_session",
		})
	case "UNLOCK_CONNECTOR":
		a.emitEvent(Event{
			Type:      "unplug",
			Timestamp: time.Now().UTC(),
			ChargerID: payload.ChargerID,
			EvseUID:   payload.EvseUID,
			Message:   "command_unlock_connector",
		})
	default:
		response.Result = "NOT_SUPPORTED"
	}

	writeOCPI(w, http.StatusOK, ocpiResponse(response))
}

func chargersToLocations(chargers []store.Charger) []ocpi.Location {
	locations := make([]ocpi.Location, 0, len(chargers))
	for _, charger := range chargers {
		locations = append(locations, ocpi.Location{
			ID:      charger.LocationID,
			Name:    charger.Name,
			Address: charger.Address,
			City:    charger.City,
			Country: charger.Country,
			Coordinates: ocpi.Coordinates{
				Latitude:  charger.Latitude,
				Longitude: charger.Longitude,
			},
			Evse: []ocpi.EVSE{
				{
					UID:    charger.EvseUID,
					Status: charger.Status,
					Connectors: []ocpi.Connector{
						{
							ID:          charger.ConnectorID,
							Standard:    "IEC_62196_T2",
							Format:      "SOCKET",
							PowerType:   "AC_3_PHASE",
							MaxVoltage:  charger.MaxVoltage,
							MaxAmperage: charger.MaxAmperage,
						},
					},
					LastUpdated: time.Now().UTC(),
				},
			},
			LastUpdated: time.Now().UTC(),
		})
	}
	return locations
}

func mapSession(session store.Session) ocpi.Session {
	var end *time.Time
	if !session.EndedAt.IsZero() {
		end = &session.EndedAt
	}
	return ocpi.Session{
		ID:          session.ID,
		Status:      session.Status,
		Kwh:         session.Kwh,
		LocationID:  session.LocationID,
		EvseUID:     session.EvseUID,
		StartDateTime: session.StartedAt,
		EndDateTime:   end,
		LastUpdated:   time.Now().UTC(),
	}
}

func writeOCPI(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeOCPI(w http.ResponseWriter, r *http.Request, dest any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dest); err != nil {
		if err == io.EOF {
			return nil
		}
		http.Error(w, "invalid json", http.StatusBadRequest)
		return err
	}
	return nil
}
