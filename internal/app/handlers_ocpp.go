package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ocpi-simulator/internal/ws"
)

type ocppCall struct {
	MessageType int
	UniqueID    string
	Action      string
	Payload     json.RawMessage
}

func (a *App) handleOCPP(w http.ResponseWriter, r *http.Request, protocol, chargePointID string) {
	if protocol != "1.6" && protocol != "2.0.1" {
		http.NotFound(w, r)
		return
	}

	conn, rw, err := ws.Upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	wsConn := ws.NewConn(conn, rw)

	for {
		msg, err := wsConn.ReadText()
		if err != nil {
			return
		}

		call, ok := parseOCPPCall(msg)
		if !ok {
			continue
		}

		if strings.EqualFold(call.Action, "BootNotification") {
			chargerID := chargePointID
			if charger, found := a.fleet.FindByOCPPIdentity(chargePointID); found {
				chargerID = charger.ChargerID
			} else if charger, found := a.store.FindChargerByChargePointID(chargePointID); found {
				chargerID = charger.ID
			}
			response := map[string]any{
				"status":      "Accepted",
				"currentTime": time.Now().UTC().Format(time.RFC3339),
				"interval":    300,
			}

			reply := []any{3, call.UniqueID, response}
			payload, _ := json.Marshal(reply)
			_ = wsConn.WriteText(string(payload))

			a.emitEvent(Event{
				Type:      "boot_notification",
				Timestamp: time.Now().UTC(),
				Message:   "boot_notification_accepted",
				ChargerID: chargerID,
			})
		}
	}
}

func parseOCPPCall(msg string) (ocppCall, bool) {
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		return ocppCall{}, false
	}
	if len(raw) < 3 {
		return ocppCall{}, false
	}

	var messageType int
	if err := json.Unmarshal(raw[0], &messageType); err != nil {
		return ocppCall{}, false
	}
	if messageType != 2 {
		return ocppCall{}, false
	}

	var uniqueID string
	if err := json.Unmarshal(raw[1], &uniqueID); err != nil {
		return ocppCall{}, false
	}

	var action string
	if err := json.Unmarshal(raw[2], &action); err != nil {
		return ocppCall{}, false
	}

	payload := json.RawMessage(nil)
	if len(raw) > 3 {
		payload = raw[3]
	}

	return ocppCall{
		MessageType: messageType,
		UniqueID:    uniqueID,
		Action:      action,
		Payload:     payload,
	}, true
}
