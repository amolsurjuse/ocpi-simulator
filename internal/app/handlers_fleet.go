package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ocpi-simulator/internal/fleet"
)

type createChargerResponse struct {
	ChargerID string `json:"chargerId"`
	Status    string `json:"status"`
	Links     struct {
		Self    string `json:"self"`
		Connect string `json:"connect"`
	} `json:"links"`
}

type deleteChargerResponse struct {
	ChargerID string `json:"chargerId"`
	Status    string `json:"status"`
}

type listChargersResponse struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"nextCursor"`
}

type updateStatusResponse struct {
	ChargerID string `json:"chargerId"`
	Status    string `json:"status"`
}

type connectionResponse struct {
	ChargerID       string `json:"chargerId"`
	ConnectionState string `json:"connectionState"`
	Remote          string `json:"remote,omitempty"`
	Since           string `json:"since,omitempty"`
}

type tapResponse struct {
	ChargerID       string `json:"chargerId"`
	ConnectorID     int    `json:"connectorId"`
	Result          string `json:"result"`
	AuthorizationID string `json:"authorizationId"`
}

type startChargingResponse struct {
	TransactionID string `json:"transactionId"`
	Status        string `json:"status"`
}

type stopChargingResponse struct {
	TransactionID string `json:"transactionId"`
	Status        string `json:"status"`
}

type meterSendResponse struct {
	Status string `json:"status"`
}

type statsResponse struct {
	ChargersTotal    int `json:"chargersTotal"`
	Connected        int `json:"connected"`
	Connecting       int `json:"connecting"`
	Disconnected     int `json:"disconnected"`
	MsgRateInPerSec  int `json:"msgRateInPerSec"`
	MsgRateOutPerSec int `json:"msgRateOutPerSec"`
}

func (a *App) handleV1(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 0 {
		http.NotFound(w, r)
		return
	}

	switch segments[0] {
	case "chargers":
		a.handleV1Chargers(w, r, segments[1:])
		return
	case "environment":
		if len(segments) == 3 && segments[1] == "chargers" && segments[2] == "import" && r.Method == http.MethodPost {
			a.handleImportEnvironmentChargers(w, r)
			return
		}
	case "stats":
		if r.Method == http.MethodGet {
			a.handleStats(w, r)
			return
		}
	case "events":
		if len(segments) == 2 && segments[1] == "stream" && r.Method == http.MethodGet {
			a.handleEventStream(w, r)
			return
		}
	}

	http.NotFound(w, r)
}

func (a *App) handleV1Chargers(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 0 {
		switch r.Method {
		case http.MethodPost:
			a.handleCreateChargerV1(w, r)
		case http.MethodGet:
			a.handleListChargersV1(w, r)
		default:
			http.NotFound(w, r)
		}
		return
	}

	if segments[0] == "bulk" {
		a.handleBulk(w, r, segments[1:])
		return
	}

	chargerID := segments[0]
	if len(segments) == 1 {
		switch r.Method {
		case http.MethodGet:
			a.handleGetChargerV1(w, r, chargerID)
		case http.MethodDelete:
			a.handleDeleteChargerV1(w, r, chargerID)
		default:
			http.NotFound(w, r)
		}
		return
	}

	switch segments[1] {
	case "config":
		if r.Method == http.MethodPatch {
			a.handlePatchConfig(w, r, chargerID)
			return
		}
	case "connection":
		a.handleConnectionLifecycle(w, r, chargerID, segments[2:])
		return
	case "connectors":
		a.handleConnectorOps(w, r, chargerID, segments[2:])
		return
	case "heartbeat":
		a.handleHeartbeatOps(w, r, chargerID, segments[2:])
		return
	case "ocpp":
		if len(segments) == 3 && segments[2] == "send" && r.Method == http.MethodPost {
			a.handleOcppSend(w, r, chargerID)
			return
		}
	}

	http.NotFound(w, r)
}

func (a *App) handleCreateChargerV1(w http.ResponseWriter, r *http.Request) {
	var payload fleet.Charger
	if err := decodeJSON(w, r, &payload); err != nil {
		return
	}

	charger, err := a.fleet.AddCharger(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := createChargerResponse{ChargerID: charger.ChargerID, Status: "CREATED"}
	resp.Links.Self = "/api/v1/chargers/" + charger.ChargerID
	resp.Links.Connect = "/api/v1/chargers/" + charger.ChargerID + "/connection/connect"

	a.emitFleetEvent(fleet.Event{
		Type:      "CHARGER_CREATED",
		Timestamp: time.Now().UTC(),
		ChargerID: charger.ChargerID,
	})

	respondJSON(w, http.StatusCreated, resp)
}

func (a *App) handleDeleteChargerV1(w http.ResponseWriter, r *http.Request, chargerID string) {
	force := r.URL.Query().Get("force") == "true"
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if len(charger.Runtime.ActiveTransactions) > 0 && !force {
		http.Error(w, "active transactions present", http.StatusConflict)
		return
	}

	if !a.fleet.RemoveCharger(chargerID) {
		http.NotFound(w, r)
		return
	}
	a.resetEventStep(chargerID)

	a.emitFleetEvent(fleet.Event{
		Type:      "CHARGER_DELETED",
		Timestamp: time.Now().UTC(),
		ChargerID: chargerID,
	})

	respondJSON(w, http.StatusOK, deleteChargerResponse{ChargerID: chargerID, Status: "DELETED"})
}

func (a *App) handleListChargersV1(w http.ResponseWriter, r *http.Request) {
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	versionFilter := strings.TrimSpace(r.URL.Query().Get("ocppVersion"))
	limit := parseLimit(r.URL.Query().Get("limit"), 100)
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))

	chargers := a.fleet.ListChargers()
	items := []map[string]any{}
	start := 0
	if cursor != "" {
		for i, charger := range chargers {
			if charger.ChargerID == cursor {
				start = i + 1
				break
			}
		}
	}

	count := 0
	var nextCursor *string
	for i := start; i < len(chargers); i++ {
		charger := chargers[i]
		if statusFilter != "" && !strings.EqualFold(charger.ConnectionState, statusFilter) {
			continue
		}
		if versionFilter != "" && !strings.EqualFold(charger.OCPPVersion, versionFilter) {
			continue
		}
		item := map[string]any{
			"chargerId":          charger.ChargerID,
			"ocppIdentity":       charger.OCPPIdentity,
			"ocppVersion":        charger.OCPPVersion,
			"connectionState":    charger.ConnectionState,
			"activeTransactions": len(charger.Runtime.ActiveTransactions),
		}
		items = append(items, item)
		count++
		if count == limit {
			last := charger.ChargerID
			nextCursor = &last
			break
		}
	}

	respondJSON(w, http.StatusOK, listChargersResponse{Items: items, NextCursor: nextCursor})
}

func (a *App) handleGetChargerV1(w http.ResponseWriter, r *http.Request, chargerID string) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	respondJSON(w, http.StatusOK, charger)
}

func (a *App) handlePatchConfig(w http.ResponseWriter, r *http.Request, chargerID string) {
	var payload map[string]any
	if err := decodeJSON(w, r, &payload); err != nil {
		return
	}

	_, err := a.fleet.PatchConfig(chargerID, func(cfg *fleet.ChargerConfig) {
		applyConfigPatch(cfg, payload)
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}

	a.emitFleetEvent(fleet.Event{Type: "CONFIG_UPDATED", Timestamp: time.Now().UTC(), ChargerID: chargerID})
	respondJSON(w, http.StatusOK, updateStatusResponse{ChargerID: chargerID, Status: "UPDATED"})
}

func (a *App) handleConnectionLifecycle(w http.ResponseWriter, r *http.Request, chargerID string, segments []string) {
	if len(segments) == 0 && r.Method == http.MethodGet {
		a.handleConnectionStatus(w, r, chargerID)
		return
	}
	if len(segments) != 1 {
		http.NotFound(w, r)
		return
	}

	switch segments[0] {
	case "connect":
		if r.Method == http.MethodPost {
			a.handleConnectCharger(w, r, chargerID)
			return
		}
	case "disconnect":
		if r.Method == http.MethodPost {
			a.handleDisconnectCharger(w, r, chargerID)
			return
		}
	}

	http.NotFound(w, r)
}

func (a *App) handleConnectCharger(w http.ResponseWriter, r *http.Request, chargerID string) {
	var payload struct {
		CSMSURL         string                `json:"csmsUrl"`
		ReconnectPolicy fleet.ReconnectPolicy `json:"reconnectPolicy"`
	}
	_ = decodeJSON(w, r, &payload)

	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	targetURL := strings.TrimSpace(payload.CSMSURL)
	if targetURL == "" {
		targetURL = strings.TrimSpace(charger.Transport.CSMSURL)
	}
	if targetURL == "" || strings.Contains(targetURL, "example.com") {
		targetURL = a.defaultSimulatorOCPPURL(charger)
	}
	charger.Transport.CSMSURL = targetURL
	_, _ = a.fleet.UpdateCharger(charger)

	now := time.Now().UTC()
	charger.ConnectionState = "CONNECTING"
	charger.ConnectionSince = &now
	charger.ConnectionRemote = hostFromURL(targetURL)
	_, _ = a.fleet.UpdateCharger(charger)

	a.emitFleetEvent(fleet.Event{Type: "CONNECTING", Timestamp: now, ChargerID: chargerID})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		connectErr := a.wsConnector.Connect(ctx, charger, targetURL)
		if connectErr != nil {
			at := time.Now().UTC()
			_, _ = a.fleet.UpdateConnectionState(chargerID, "ERROR", hostFromURL(targetURL), &at)
			a.emitFleetEvent(fleet.Event{
				Type:      "CONNECTION_ERROR",
				Timestamp: at,
				ChargerID: chargerID,
				Message:   connectErr.Error(),
			})
			return
		}

		connectedAt := time.Now().UTC()
		_, _ = a.fleet.UpdateConnectionState(chargerID, "CONNECTED", hostFromURL(targetURL), &connectedAt)
		_, _ = a.fleet.UpdateRuntime(chargerID, func(runtime *fleet.Runtime) {
			runtime.LastHeartbeatAt = &connectedAt
			runtime.LastMessageAt = &connectedAt
		})
		a.emitFleetEvent(fleet.Event{Type: "CONNECTED", Timestamp: connectedAt, ChargerID: chargerID})
		a.resetEventStep(chargerID)
	}()

	respondJSON(w, http.StatusAccepted, updateStatusResponse{ChargerID: chargerID, Status: "CONNECTING"})
}

func (a *App) handleDisconnectCharger(w http.ResponseWriter, r *http.Request, chargerID string) {
	var payload struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(w, r, &payload)
	_, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	now := time.Now().UTC()
	_, _ = a.fleet.UpdateConnectionState(chargerID, "DISCONNECTING", "", &now)
	a.emitFleetEvent(fleet.Event{Type: "DISCONNECTING", Timestamp: now, ChargerID: chargerID})

	go func(reason string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if reason == "" {
			reason = "OPERATOR_REQUEST"
		}
		if err := a.wsConnector.Disconnect(ctx, chargerID, reason); err != nil {
			at := time.Now().UTC()
			_, _ = a.fleet.UpdateConnectionState(chargerID, "ERROR", "", &at)
			a.emitFleetEvent(fleet.Event{
				Type:      "CONNECTION_ERROR",
				Timestamp: at,
				ChargerID: chargerID,
				Message:   err.Error(),
			})
			return
		}
		at := time.Now().UTC()
		_, _ = a.fleet.UpdateConnectionState(chargerID, "DISCONNECTED", "", &at)
		_, _ = a.fleet.UpdateRuntime(chargerID, func(runtime *fleet.Runtime) {
			runtime.ActiveTransactions = []fleet.Transaction{}
		})
		a.resetEventStep(chargerID)
		a.emitFleetEvent(fleet.Event{Type: "DISCONNECTED", Timestamp: at, ChargerID: chargerID})
	}(payload.Reason)

	respondJSON(w, http.StatusAccepted, updateStatusResponse{ChargerID: chargerID, Status: "DISCONNECTING"})
}

func (a *App) handleConnectionStatus(w http.ResponseWriter, r *http.Request, chargerID string) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	resp := connectionResponse{ChargerID: chargerID, ConnectionState: charger.ConnectionState, Remote: charger.ConnectionRemote}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if info, err := a.wsConnector.Connection(ctx, chargerID); err == nil {
		if info.State != "" {
			resp.ConnectionState = info.State
		}
		if info.LastMessageAt != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, info.LastMessageAt); parseErr == nil {
				_, _ = a.fleet.UpdateRuntime(chargerID, func(runtime *fleet.Runtime) {
					runtime.LastMessageAt = &parsed
				})
			}
		}
	}
	if charger.ConnectionSince != nil {
		resp.Since = charger.ConnectionSince.Format(time.RFC3339)
	}
	respondJSON(w, http.StatusOK, resp)
}

func (a *App) handleConnectorOps(w http.ResponseWriter, r *http.Request, chargerID string, segments []string) {
	if len(segments) < 2 {
		http.NotFound(w, r)
		return
	}
	connectorID, err := strconv.Atoi(segments[0])
	if err != nil {
		http.Error(w, "invalid connectorId", http.StatusBadRequest)
		return
	}

	action := segments[1]
	switch action {
	case "tap":
		if r.Method == http.MethodPost {
			a.handleTap(w, r, chargerID, connectorID)
			return
		}
	case "plug-and-charge":
		if len(segments) == 3 {
			a.handlePnC(w, r, chargerID, connectorID, segments[2])
			return
		}
	case "charging":
		if len(segments) == 3 {
			a.handleCharging(w, r, chargerID, connectorID, segments[2])
			return
		}
	case "meter-values":
		if len(segments) == 3 && segments[2] == "send" && r.Method == http.MethodPost {
			a.handleMeterSend(w, r, chargerID, connectorID)
			return
		}
	case "status":
		if r.Method == http.MethodPost {
			a.handleStatusUpdate(w, r, chargerID, connectorID)
			return
		}
	case "faults":
		if len(segments) == 3 {
			a.handleFaults(w, r, chargerID, connectorID, segments[2])
			return
		}
	}

	http.NotFound(w, r)
}

func (a *App) handleTap(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !hasConnector(&charger, connectorID) {
		http.NotFound(w, r)
		return
	}

	var payload map[string]any
	_ = decodeJSON(w, r, &payload)
	authID := fleet.NewJobID("auth")

	a.emitFleetEvent(fleet.Event{
		Type:        "AUTH_ACCEPTED",
		Timestamp:   time.Now().UTC(),
		ChargerID:   chargerID,
		ConnectorID: connectorID,
		Result:      "ACCEPTED",
		Data:        payload,
	})

	respondJSON(w, http.StatusOK, tapResponse{ChargerID: chargerID, ConnectorID: connectorID, Result: "ACCEPTED", AuthorizationID: authID})
}

func (a *App) handlePnC(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int, action string) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !hasConnector(&charger, connectorID) {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "start":
		var payload map[string]any
		_ = decodeJSON(w, r, &payload)
		correlation := fleet.NewJobID("pnc")
		a.emitFleetEvent(fleet.Event{Type: "PNC_STARTING", Timestamp: time.Now().UTC(), ChargerID: chargerID, ConnectorID: connectorID, Message: correlation, Data: payload})
		respondJSON(w, http.StatusAccepted, map[string]any{"chargerId": chargerID, "connectorId": connectorID, "status": "PNC_STARTING", "correlationId": correlation})
	case "stop":
		var payload map[string]any
		_ = decodeJSON(w, r, &payload)
		a.emitFleetEvent(fleet.Event{Type: "PNC_STOPPING", Timestamp: time.Now().UTC(), ChargerID: chargerID, ConnectorID: connectorID, Data: payload})
		respondJSON(w, http.StatusAccepted, map[string]any{"chargerId": chargerID, "connectorId": connectorID, "status": "PNC_STOPPING"})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleCharging(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int, action string) {
	switch action {
	case "start":
		a.handleChargingStart(w, r, chargerID, connectorID)
	case "stop":
		a.handleChargingStop(w, r, chargerID, connectorID)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleChargingStart(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !hasConnector(&charger, connectorID) {
		http.NotFound(w, r)
		return
	}

	var payload struct {
		Auth struct {
			IdTag           string `json:"idTag"`
			AuthorizationID string `json:"authorizationId"`
		} `json:"auth"`
		MeterStartWh    int64 `json:"meterStartWh"`
		TargetPowerW    int64 `json:"targetPowerW"`
		ChargingProfile struct {
			Enabled bool `json:"enabled"`
		} `json:"chargingProfile"`
	}
	_ = decodeJSON(w, r, &payload)

	transaction := fleet.Transaction{
		TransactionID:   fleet.NewJobID("tx"),
		ConnectorID:     connectorID,
		Status:          "STARTED",
		MeterStartWh:    payload.MeterStartWh,
		MeterStopWh:     payload.MeterStartWh,
		StartedAt:       time.Now().UTC(),
		AuthorizationID: payload.Auth.AuthorizationID,
		IdTag:           payload.Auth.IdTag,
	}

	_, _ = a.fleet.UpdateRuntime(chargerID, func(runtime *fleet.Runtime) {
		runtime.ActiveTransactions = append(runtime.ActiveTransactions, transaction)
		runtime.LastMessageAt = &transaction.StartedAt
	})

	a.emitFleetEvent(fleet.Event{Type: "TX_STARTED", Timestamp: time.Now().UTC(), ChargerID: chargerID, ConnectorID: connectorID, TransactionID: transaction.TransactionID})
	a.emitStatusNotification(charger, connectorID, "Charging", "NoError", time.Now().UTC())
	respondJSON(w, http.StatusOK, startChargingResponse{TransactionID: transaction.TransactionID, Status: "STARTED"})
}

func (a *App) handleChargingStop(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var payload struct {
		TransactionID string `json:"transactionId"`
		Reason        string `json:"reason"`
		MeterStopWh   int64  `json:"meterStopWh"`
	}
	_ = decodeJSON(w, r, &payload)

	var stoppedTxID string
	stoppedAt := time.Now().UTC()
	_, _ = a.fleet.UpdateRuntime(chargerID, func(runtime *fleet.Runtime) {
		for i, tx := range runtime.ActiveTransactions {
			if tx.TransactionID != payload.TransactionID {
				continue
			}
			stoppedTxID = tx.TransactionID
			runtime.ActiveTransactions = append(runtime.ActiveTransactions[:i], runtime.ActiveTransactions[i+1:]...)
			runtime.LastMessageAt = &stoppedAt
			return
		}
	})

	if stoppedTxID == "" {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}

	a.emitFleetEvent(fleet.Event{Type: "TX_STOPPED", Timestamp: stoppedAt, ChargerID: chargerID, ConnectorID: connectorID, TransactionID: stoppedTxID, Message: payload.Reason})
	a.emitStatusNotification(charger, connectorID, "Available", "NoError", stoppedAt)
	respondJSON(w, http.StatusOK, stopChargingResponse{TransactionID: stoppedTxID, Status: "STOPPED"})
}

func (a *App) handleMeterSend(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !hasConnector(&charger, connectorID) {
		http.NotFound(w, r)
		return
	}

	var payload map[string]any
	_ = decodeJSON(w, r, &payload)
	payload["connectorId"] = connectorID
	payload["chargerId"] = chargerID

	a.emitFleetEvent(fleet.Event{Type: "METER_VALUES", Timestamp: time.Now().UTC(), ChargerID: chargerID, ConnectorID: connectorID, Data: payload})
	respondJSON(w, http.StatusAccepted, meterSendResponse{Status: "QUEUED"})
}

func (a *App) handleStatusUpdate(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var payload struct {
		Status    string `json:"status"`
		ErrorCode string `json:"errorCode"`
	}
	_ = decodeJSON(w, r, &payload)

	if !updateConnector(&charger, connectorID, func(conn *fleet.Connector) {
		if payload.Status != "" {
			conn.Status = payload.Status
		}
		if payload.ErrorCode != "" {
			conn.ErrorCode = payload.ErrorCode
		}
	}) {
		http.NotFound(w, r)
		return
	}
	_, _ = a.fleet.UpdateCharger(charger)

	a.emitFleetEvent(fleet.Event{Type: "STATUS_UPDATED", Timestamp: time.Now().UTC(), ChargerID: chargerID, ConnectorID: connectorID})
	respondJSON(w, http.StatusOK, map[string]string{"status": "UPDATED"})
}

func (a *App) handleFaults(w http.ResponseWriter, r *http.Request, chargerID string, connectorID int, action string) {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "inject":
		var payload struct {
			Type        string `json:"type"`
			ErrorCode   string `json:"errorCode"`
			DurationSec int    `json:"durationSec"`
			Disconnect  bool   `json:"disconnect"`
		}
		_ = decodeJSON(w, r, &payload)

		if !updateConnector(&charger, connectorID, func(conn *fleet.Connector) {
			conn.Status = "Faulted"
			conn.ErrorCode = payload.ErrorCode
			conn.FaultType = payload.Type
		}) {
			http.NotFound(w, r)
			return
		}
		_, _ = a.fleet.UpdateCharger(charger)

		a.emitFleetEvent(fleet.Event{Type: "FAULTED", Timestamp: time.Now().UTC(), ChargerID: chargerID, ConnectorID: connectorID, Message: payload.Type})
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "FAULT_INJECTED"})
	case "clear":
		if !updateConnector(&charger, connectorID, func(conn *fleet.Connector) {
			conn.Status = "Available"
			conn.ErrorCode = "NoError"
			conn.FaultType = ""
		}) {
			http.NotFound(w, r)
			return
		}
		_, _ = a.fleet.UpdateCharger(charger)
		a.emitFleetEvent(fleet.Event{Type: "RECOVERED", Timestamp: time.Now().UTC(), ChargerID: chargerID, ConnectorID: connectorID})
		respondJSON(w, http.StatusOK, map[string]string{"status": "CLEARED"})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleHeartbeatOps(w http.ResponseWriter, r *http.Request, chargerID string, segments []string) {
	if len(segments) == 1 && segments[0] == "send" && r.Method == http.MethodPost {
		a.handleHeartbeatSend(w, r, chargerID)
		return
	}
	if len(segments) == 1 && segments[0] == "interval" && r.Method == http.MethodPost {
		a.handleHeartbeatInterval(w, r, chargerID)
		return
	}
	http.NotFound(w, r)
}

func (a *App) handleHeartbeatSend(w http.ResponseWriter, r *http.Request, chargerID string) {
	now := time.Now().UTC()
	_, err := a.fleet.UpdateRuntime(chargerID, func(runtime *fleet.Runtime) {
		runtime.LastHeartbeatAt = &now
		runtime.LastMessageAt = &now
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.emitFleetEvent(fleet.Event{Type: "HEARTBEAT", Timestamp: now, ChargerID: chargerID})
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "QUEUED"})
}

func (a *App) handleHeartbeatInterval(w http.ResponseWriter, r *http.Request, chargerID string) {
	var payload struct {
		HeartbeatIntervalSec int `json:"heartbeatIntervalSec"`
	}
	_ = decodeJSON(w, r, &payload)

	_, err := a.fleet.PatchConfig(chargerID, func(cfg *fleet.ChargerConfig) {
		if payload.HeartbeatIntervalSec > 0 {
			cfg.HeartbeatIntervalSec = payload.HeartbeatIntervalSec
		}
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "UPDATED"})
}

func (a *App) handleOcppSend(w http.ResponseWriter, r *http.Request, chargerID string) {
	var payload struct {
		Action        string         `json:"action"`
		Payload       map[string]any `json:"payload"`
		AwaitResponse bool           `json:"awaitResponse"`
		TimeoutMs     int            `json:"timeoutMs"`
	}
	_ = decodeJSON(w, r, &payload)

	response := map[string]any{}
	if strings.EqualFold(payload.Action, "BootNotification") {
		response = map[string]any{
			"status":      "Accepted",
			"currentTime": time.Now().UTC().Format(time.RFC3339),
			"interval":    60,
		}
	}

	a.emitFleetEvent(fleet.Event{Type: "OCPP_SENT", Timestamp: time.Now().UTC(), ChargerID: chargerID, Message: payload.Action, Data: payload.Payload})
	respondJSON(w, http.StatusOK, map[string]any{"result": "OK", "response": response})
}

func (a *App) handleBulk(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 0 {
		if r.Method == http.MethodPost {
			a.handleBulkAdd(w, r)
			return
		}
	}
	if len(segments) == 1 {
		switch segments[0] {
		case "connect":
			if r.Method == http.MethodPost {
				a.handleBulkConnect(w, r)
				return
			}
		case "disconnect":
			if r.Method == http.MethodPost {
				a.handleBulkDisconnect(w, r)
				return
			}
		}
	}

	http.NotFound(w, r)
}

func (a *App) handleBulkAdd(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Count                   int               `json:"count"`
		IDTemplate              string            `json:"idTemplate"`
		IdentityTemplate        string            `json:"identityTemplate"`
		OCPPVersionDistribution map[string]int    `json:"ocppVersionDistribution"`
		BaseConfig              map[string]any    `json:"baseConfig"`
		Transport               fleet.Transport   `json:"transport"`
		Tags                    map[string]string `json:"tags"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		return
	}
	if payload.Count <= 0 {
		http.Error(w, "count must be > 0", http.StatusBadRequest)
		return
	}

	job := fleet.BulkJob{JobID: fleet.NewJobID("job"), Status: "STARTED", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Total: payload.Count}
	a.fleet.SetJob(job)

	go func() {
		for i := 0; i < payload.Count; i++ {
			chargerID := formatTemplate(payload.IDTemplate, i)
			identity := formatTemplate(payload.IdentityTemplate, i)
			version := pickVersion(payload.OCPPVersionDistribution, i)

			charger := fleet.Charger{
				ChargerID:    chargerID,
				OCPPIdentity: identity,
				OCPPVersion:  version,
				Transport:    payload.Transport,
				Tags:         payload.Tags,
			}
			applyBaseConfig(&charger, payload.BaseConfig)
			_, _ = a.fleet.AddCharger(charger)

			_, _ = a.fleet.UpdateJob(job.JobID, func(j *fleet.BulkJob) {
				j.Completed++
				if j.Completed >= j.Total {
					j.Status = "COMPLETED"
				}
			})
		}
	}()

	respondJSON(w, http.StatusAccepted, map[string]any{"jobId": job.JobID, "status": job.Status})
}

func (a *App) handleBulkConnect(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Filter map[string]string `json:"filter"`
		RampUp struct {
			RatePerSec int `json:"ratePerSec"`
		} `json:"rampUp"`
	}
	_ = decodeJSON(w, r, &payload)

	chargers := filterChargers(a.fleet.ListChargers(), payload.Filter)
	rate := payload.RampUp.RatePerSec
	if rate <= 0 {
		rate = 100
	}
	interval := time.Second / time.Duration(rate)
	if interval <= 0 {
		interval = time.Millisecond
	}

	go func() {
		for _, charger := range chargers {
			_ = a.connectChargerAsync(charger.ChargerID)
			time.Sleep(interval)
		}
	}()

	respondJSON(w, http.StatusAccepted, map[string]string{"status": "STARTED"})
}

func (a *App) handleBulkDisconnect(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Filter map[string]string `json:"filter"`
	}
	_ = decodeJSON(w, r, &payload)

	chargers := filterChargers(a.fleet.ListChargers(), payload.Filter)

	go func() {
		for _, charger := range chargers {
			_ = a.disconnectChargerAsync(charger.ChargerID)
		}
	}()

	respondJSON(w, http.StatusAccepted, map[string]string{"status": "STARTED"})
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	chargers := a.fleet.ListChargers()
	stats := statsResponse{ChargersTotal: len(chargers)}
	for _, charger := range chargers {
		switch strings.ToUpper(charger.ConnectionState) {
		case "CONNECTED":
			stats.Connected++
		case "CONNECTING":
			stats.Connecting++
		case "DISCONNECTED", "DISCONNECTING":
			stats.Disconnected++
		}
	}

	stats.MsgRateOutPerSec = a.fleetHub.Metrics().RatePerSec()
	stats.MsgRateInPerSec = 0

	respondJSON(w, http.StatusOK, stats)
}

func (a *App) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	chargerFilter := strings.TrimSpace(r.URL.Query().Get("chargerId"))
	eventCh := a.fleetHub.Subscribe()
	defer a.fleetHub.Unsubscribe(eventCh)

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-eventCh:
			if chargerFilter != "" && event.ChargerID != chargerFilter {
				continue
			}
			payload, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (a *App) emitFleetEvent(event fleet.Event) {
	a.fleetHub.Publish(event)
	payload, err := json.Marshal(event)
	if err == nil {
		a.hub.Broadcast(payload)
	}

	go func(evt fleet.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := a.wsConnector.LogEvent(ctx, evt); err != nil {
			a.log.Warn("failed to forward fleet event", "chargerId", evt.ChargerID, "eventType", evt.Type, "error", err)
		}
	}(event)
}

func parseLimit(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > 1000 {
		return 1000
	}
	return parsed
}

func applyConfigPatch(cfg *fleet.ChargerConfig, patch map[string]any) {
	if v, ok := patch["heartbeatIntervalSec"].(float64); ok {
		cfg.HeartbeatIntervalSec = int(v)
	}
	if v, ok := patch["meterValuesIntervalSec"].(float64); ok {
		cfg.MeterValuesIntervalSec = int(v)
	}
	if soc, ok := patch["soc"].(map[string]any); ok {
		if v, ok := soc["startPercent"].(float64); ok {
			cfg.SOC.StartPercent = int(v)
		}
		if v, ok := soc["endPercent"].(float64); ok {
			cfg.SOC.EndPercent = int(v)
		}
		if v, ok := soc["ratePercentPerMin"].(float64); ok {
			cfg.SOC.RatePercentPerMin = v
		}
		if v, ok := soc["enabled"].(bool); ok {
			cfg.SOC.Enabled = v
		}
	}
}

func updateConnector(charger *fleet.Charger, connectorID int, update func(conn *fleet.Connector)) bool {
	for i := range charger.Connectors {
		if charger.Connectors[i].ConnectorID == connectorID {
			update(&charger.Connectors[i])
			return true
		}
	}
	return false
}

func hasConnector(charger *fleet.Charger, connectorID int) bool {
	for _, conn := range charger.Connectors {
		if conn.ConnectorID == connectorID {
			return true
		}
	}
	return false
}

func hostFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func (a *App) defaultSimulatorOCPPURL(charger fleet.Charger) string {
	parsed, err := url.Parse(a.cfg.BaseURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := "ws"
	if parsed.Scheme == "https" {
		scheme = "wss"
	}
	protocol := "1.6"
	if strings.EqualFold(charger.OCPPVersion, "OCPP201") || strings.EqualFold(charger.OCPPVersion, "2.0.1") {
		protocol = "2.0.1"
	}
	identity := charger.OCPPIdentity
	if identity == "" {
		identity = charger.ChargerID
	}
	return fmt.Sprintf("%s://%s/ocpp/%s/%s", scheme, parsed.Host, protocol, identity)
}

func formatTemplate(template string, idx int) string {
	if template == "" {
		return fmt.Sprintf("sim-%06d", idx+1)
	}
	if strings.Contains(template, "%") {
		return fmt.Sprintf(template, idx+1)
	}
	return template + strconv.Itoa(idx+1)
}

func pickVersion(dist map[string]int, idx int) string {
	if len(dist) == 0 {
		return "OCPP16J"
	}
	total := 0
	for _, v := range dist {
		total += v
	}
	if total == 0 {
		return "OCPP16J"
	}

	bucket := (idx % total) + 1
	cumulative := 0
	for version, count := range dist {
		cumulative += count
		if bucket <= cumulative {
			return version
		}
	}
	for version := range dist {
		return version
	}
	return "OCPP16J"
}

func applyBaseConfig(charger *fleet.Charger, base map[string]any) {
	if base == nil {
		return
	}
	if v, ok := base["heartbeatIntervalSec"].(float64); ok {
		charger.Config.HeartbeatIntervalSec = int(v)
	}
	if v, ok := base["meterValuesIntervalSec"].(float64); ok {
		charger.Config.MeterValuesIntervalSec = int(v)
	}
}

func filterChargers(chargers []fleet.Charger, filter map[string]string) []fleet.Charger {
	if len(filter) == 0 {
		return chargers
	}
	items := []fleet.Charger{}
	for _, charger := range chargers {
		if version, ok := filter["ocppVersion"]; ok {
			if !strings.EqualFold(charger.OCPPVersion, version) {
				continue
			}
		}
		match := true
		for key, value := range filter {
			if strings.HasPrefix(key, "tag.") {
				tagKey := strings.TrimPrefix(key, "tag.")
				if charger.Tags == nil || charger.Tags[tagKey] != value {
					match = false
					break
				}
			}
		}
		if match {
			items = append(items, charger)
		}
	}
	return items
}

func (a *App) connectChargerAsync(chargerID string) error {
	now := time.Now().UTC()
	_, err := a.fleet.UpdateConnectionState(chargerID, "CONNECTING", "", &now)
	if err != nil {
		return err
	}
	a.emitFleetEvent(fleet.Event{Type: "CONNECTING", Timestamp: now, ChargerID: chargerID})

	go func() {
		time.Sleep(150 * time.Millisecond)
		at := time.Now().UTC()
		_, _ = a.fleet.UpdateConnectionState(chargerID, "CONNECTED", "", &at)
		a.emitFleetEvent(fleet.Event{Type: "CONNECTED", Timestamp: at, ChargerID: chargerID})
	}()
	return nil
}

func (a *App) disconnectChargerAsync(chargerID string) error {
	now := time.Now().UTC()
	_, err := a.fleet.UpdateConnectionState(chargerID, "DISCONNECTING", "", &now)
	if err != nil {
		return err
	}
	a.emitFleetEvent(fleet.Event{Type: "DISCONNECTING", Timestamp: now, ChargerID: chargerID})

	go func() {
		time.Sleep(150 * time.Millisecond)
		at := time.Now().UTC()
		_, _ = a.fleet.UpdateConnectionState(chargerID, "DISCONNECTED", "", &at)
		a.emitFleetEvent(fleet.Event{Type: "DISCONNECTED", Timestamp: at, ChargerID: chargerID})
	}()
	return nil
}
