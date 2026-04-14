package app

import (
	"context"
	"hash/fnv"
	"time"

	"ocpi-simulator/internal/fleet"
)

func (a *App) runFleetEventLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.EventInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.tickFleetEvents()
		}
	}
}

func (a *App) tickFleetEvents() {
	chargers := a.fleet.ListChargers()
	now := time.Now().UTC()

	for _, charger := range chargers {
		if len(charger.Connectors) == 0 {
			continue
		}

		connectorID := charger.Connectors[0].ConnectorID

		if len(charger.Runtime.ActiveTransactions) > 0 {
			tx := charger.Runtime.ActiveTransactions[0]
			if now.Sub(tx.StartedAt) >= 4*a.cfg.EventInterval {
				txID := a.stopSimTransaction(charger, now)
				if txID != "" {
					a.emitFleetEvent(fleet.Event{
						Type:          "STOP",
						Timestamp:     now,
						ChargerID:     charger.ChargerID,
						ConnectorID:   connectorID,
						TransactionID: txID,
						Message:       "ev_disconnected",
					})
					a.emitStatusNotification(charger, connectorID, "Finishing", "NoError", now)
					a.emitStopTransaction(charger, connectorID, txID, tx.MeterStopWh, "EVDisconnected", now)
					a.emitStatusNotification(charger, connectorID, "Available", "NoError", now)
				}
			} else {
				txID, meterWh := a.sendSimMeterValue(charger, now)
				if txID != "" {
					a.emitStatusNotification(charger, connectorID, "Charging", "NoError", now)
					a.emitMeterTelemetry(charger, connectorID, txID, meterWh, now)
				}
			}
			continue
		}

		if charger.ConnectionState != "CONNECTED" {
			continue
		}

		step := a.nextEventStep(charger.ChargerID)

		_, _ = a.fleet.UpdateRuntime(charger.ChargerID, func(runtime *fleet.Runtime) {
			runtime.LastMessageAt = &now
		})

		switch step {
		case 0:
			_, _ = a.fleet.UpdateRuntime(charger.ChargerID, func(runtime *fleet.Runtime) {
				runtime.LastHeartbeatAt = &now
			})
			a.emitFleetEvent(fleet.Event{
				Type:        "DMS",
				Timestamp:   now,
				ChargerID:   charger.ChargerID,
				ConnectorID: connectorID,
				Message:     "device_management_sync",
			})
			a.emitStatusNotification(charger, connectorID, "Available", "NoError", now)
		case 1:
			txID := a.startSimTransaction(charger, connectorID, now)
			a.emitFleetEvent(fleet.Event{
				Type:          "START",
				Timestamp:     now,
				ChargerID:     charger.ChargerID,
				ConnectorID:   connectorID,
				TransactionID: txID,
			})
			a.emitStatusNotification(charger, connectorID, "Preparing", "NoError", now)
			a.emitStartTransaction(charger, connectorID, txID, now)
		case 2:
			txID, meterWh := a.sendSimMeterValue(charger, now)
			if txID != "" {
				a.emitStatusNotification(charger, connectorID, "Charging", "NoError", now)
				a.emitMeterTelemetry(charger, connectorID, txID, meterWh, now)
			}
		case 3:
			meterStopWh := int64(charger.Config.Metering.EnergyWhStart)
			if len(charger.Runtime.ActiveTransactions) > 0 {
				meterStopWh = charger.Runtime.ActiveTransactions[0].MeterStopWh
			}
			txID := a.stopSimTransaction(charger, now)
			if txID != "" {
				a.emitFleetEvent(fleet.Event{
					Type:          "STOP",
					Timestamp:     now,
					ChargerID:     charger.ChargerID,
					ConnectorID:   connectorID,
					TransactionID: txID,
					Message:       "completed",
				})
				a.emitStatusNotification(charger, connectorID, "Finishing", "NoError", now)
				a.emitStopTransaction(charger, connectorID, txID, meterStopWh, "Local", now)
			}
		case 4:
			a.emitFleetEvent(fleet.Event{
				Type:        "UNPLUG",
				Timestamp:   now,
				ChargerID:   charger.ChargerID,
				ConnectorID: connectorID,
				Message:     "ev_disconnected",
			})
			a.emitStatusNotification(charger, connectorID, "Available", "NoError", now)
		}
	}
}

func (a *App) emitStartTransaction(charger fleet.Charger, connectorID int, txID string, now time.Time) {
	action := "StartTransaction"
	a.forwardOCPPAction(charger, action, buildStartTransactionPayload(charger, connectorID, txID, now))
}

func (a *App) emitStopTransaction(charger fleet.Charger, connectorID int, txID string, meterStopWh int64, reason string, now time.Time) {
	action := "StopTransaction"
	a.forwardOCPPAction(charger, action, buildStopTransactionPayload(charger, connectorID, txID, meterStopWh, reason, now))
}

func (a *App) emitStatusNotification(charger fleet.Charger, connectorID int, status string, errorCode string, now time.Time) {
	action := "StatusNotification"
	data := map[string]any{
		"ocppVersion": charger.OCPPVersion,
		"status":      status,
		"errorCode":   errorCode,
	}
	if charger.OCPPVersion == "OCPP201" {
		action = "TransactionEvent"
		data["eventType"] = "Updated"
		data["triggerReason"] = "ChargingStateChanged"
		data["chargingState"] = status
	}
	data["ocppAction"] = action

	a.emitFleetEvent(fleet.Event{
		Type:        "STATUS_NOTIFICATION",
		Timestamp:   now,
		ChargerID:   charger.ChargerID,
		ConnectorID: connectorID,
		Data:        data,
	})

	a.forwardOCPPAction(charger, action, buildStatusPayload(charger, connectorID, status, errorCode, now))
}

func (a *App) emitMeterTelemetry(charger fleet.Charger, connectorID int, txID string, meterWh int64, now time.Time) {
	action := "MeterValues"
	data := map[string]any{
		"ocppVersion": charger.OCPPVersion,
		"meterWh":     meterWh,
	}

	if charger.OCPPVersion == "OCPP201" {
		action = "TransactionEvent"
		data["ocppAction"] = action
		data["eventType"] = "Updated"
		data["triggerReason"] = "MeterValuePeriodic"
	} else {
		data["ocppAction"] = action
	}

	a.emitFleetEvent(fleet.Event{
		Type:          "METER_VALUE",
		Timestamp:     now,
		ChargerID:     charger.ChargerID,
		ConnectorID:   connectorID,
		TransactionID: txID,
		Data:          data,
	})

	a.forwardOCPPAction(charger, action, buildMeterPayload(charger, connectorID, txID, meterWh, now))
}

func (a *App) nextEventStep(chargerID string) int {
	a.eventCycleMu.Lock()
	defer a.eventCycleMu.Unlock()
	current := a.eventCycle[chargerID]
	a.eventCycle[chargerID] = (current + 1) % 5
	return current
}

func (a *App) resetEventStep(chargerID string) {
	a.eventCycleMu.Lock()
	defer a.eventCycleMu.Unlock()
	delete(a.eventCycle, chargerID)
}

func (a *App) startSimTransaction(charger fleet.Charger, connectorID int, now time.Time) string {
	if len(charger.Runtime.ActiveTransactions) > 0 {
		return charger.Runtime.ActiveTransactions[0].TransactionID
	}

	meterStart := charger.Config.Metering.EnergyWhStart
	if meterStart <= 0 {
		meterStart = 1200000
	}

	tx := fleet.Transaction{
		TransactionID: fleet.NewJobID("tx"),
		ConnectorID:   connectorID,
		Status:        "STARTED",
		MeterStartWh:  meterStart,
		MeterStopWh:   meterStart,
		StartedAt:     now,
	}

	_, _ = a.fleet.UpdateRuntime(charger.ChargerID, func(runtime *fleet.Runtime) {
		runtime.ActiveTransactions = append(runtime.ActiveTransactions, tx)
	})

	return tx.TransactionID
}

func (a *App) sendSimMeterValue(charger fleet.Charger, now time.Time) (string, int64) {
	if len(charger.Runtime.ActiveTransactions) == 0 {
		return "", 0
	}

	txID := charger.Runtime.ActiveTransactions[0].TransactionID
	increment := int64(25)
	if charger.Config.Metering.PowerW > 0 && a.cfg.EventInterval > 0 {
		calculated := int64(float64(charger.Config.Metering.PowerW) * a.cfg.EventInterval.Seconds() / 3600.0)
		if calculated > 0 {
			increment = calculated
		}
	}

	meterWh := charger.Runtime.ActiveTransactions[0].MeterStopWh + increment
	_, _ = a.fleet.UpdateRuntime(charger.ChargerID, func(runtime *fleet.Runtime) {
		if len(runtime.ActiveTransactions) == 0 {
			return
		}
		runtime.ActiveTransactions[0].MeterStopWh = meterWh
		runtime.LastMessageAt = &now
	})

	return txID, meterWh
}

func (a *App) stopSimTransaction(charger fleet.Charger, now time.Time) string {
	if len(charger.Runtime.ActiveTransactions) == 0 {
		return ""
	}

	txID := charger.Runtime.ActiveTransactions[0].TransactionID
	_, _ = a.fleet.UpdateRuntime(charger.ChargerID, func(runtime *fleet.Runtime) {
		if len(runtime.ActiveTransactions) == 0 {
			return
		}
		runtime.ActiveTransactions = runtime.ActiveTransactions[1:]
		runtime.LastMessageAt = &now
	})

	return txID
}

func (a *App) forwardOCPPAction(charger fleet.Charger, action string, payload map[string]any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := a.wsConnector.Send(ctx, charger.ChargerID, action, payload); err != nil {
			a.log.Warn("failed to forward ocpp action", "chargerId", charger.ChargerID, "action", action, "error", err)
		}
	}()
}

func buildStatusPayload(charger fleet.Charger, connectorID int, status, errorCode string, now time.Time) map[string]any {
	if charger.OCPPVersion == "OCPP201" {
		payload := map[string]any{
			"eventType":     "Updated",
			"timestamp":     now.Format(time.RFC3339),
			"triggerReason": "ChargingStateChanged",
			"evse": map[string]any{
				"id": connectorID,
			},
			"transactionInfo": map[string]any{
				"chargingState": status,
			},
		}
		if errorCode != "" {
			payload["customData"] = map[string]any{"errorCode": errorCode}
		}
		return payload
	}

	return map[string]any{
		"connectorId": connectorID,
		"status":      status,
		"errorCode":   errorCode,
		"timestamp":   now.Format(time.RFC3339),
	}
}

func buildMeterPayload(charger fleet.Charger, connectorID int, txID string, meterWh int64, now time.Time) map[string]any {
	numericTxID := numericTransactionID(txID)
	if charger.OCPPVersion == "OCPP201" {
		payload := map[string]any{
			"eventType":     "Updated",
			"timestamp":     now.Format(time.RFC3339),
			"triggerReason": "MeterValuePeriodic",
			"evse": map[string]any{
				"id": connectorID,
			},
			"meterValue": []map[string]any{
				{
					"timestamp": now.Format(time.RFC3339),
					"sampledValue": []map[string]any{
						{
							"value":     meterWh,
							"measurand": "Energy.Active.Import.Register",
							"unitOfMeasure": map[string]any{
								"unit": "Wh",
							},
						},
					},
				},
			},
		}
		if txID != "" {
			payload["transactionInfo"] = map[string]any{"transactionId": numericTxID}
		}
		return payload
	}

	payload := map[string]any{
		"connectorId":   connectorID,
		"transactionId": numericTxID,
		"meterValue": []map[string]any{
			{
				"timestamp": now.Format(time.RFC3339),
				"sampledValue": []map[string]any{
					{
						"value":     meterWh,
						"measurand": "Energy.Active.Import.Register",
						"unit":      "Wh",
					},
				},
			},
		},
	}
	return payload
}

func buildStartTransactionPayload(charger fleet.Charger, connectorID int, txID string, now time.Time) map[string]any {
	numericTxID := numericTransactionID(txID)
	if charger.OCPPVersion == "OCPP201" {
		return map[string]any{
			"eventType":     "Started",
			"timestamp":     now.Format(time.RFC3339),
			"triggerReason": "RemoteStart",
			"evse": map[string]any{
				"id": connectorID,
			},
			"transactionInfo": map[string]any{
				"transactionId": numericTxID,
			},
			"meterValue": []map[string]any{
				{
					"timestamp": now.Format(time.RFC3339),
					"sampledValue": []map[string]any{
						{
							"value":     charger.Config.Metering.EnergyWhStart,
							"measurand": "Energy.Active.Import.Register",
							"unitOfMeasure": map[string]any{
								"unit": "Wh",
							},
						},
					},
				},
			},
		}
	}

	return map[string]any{
		"connectorId":   connectorID,
		"idTag":         "SIMULATOR",
		"meterStart":    charger.Config.Metering.EnergyWhStart,
		"timestamp":     now.Format(time.RFC3339),
		"transactionId": numericTxID,
	}
}

func buildStopTransactionPayload(charger fleet.Charger, connectorID int, txID string, meterStopWh int64, reason string, now time.Time) map[string]any {
	numericTxID := numericTransactionID(txID)
	if charger.OCPPVersion == "OCPP201" {
		return map[string]any{
			"eventType":     "Ended",
			"timestamp":     now.Format(time.RFC3339),
			"triggerReason": "EVCommunicationLost",
			"evse": map[string]any{
				"id": connectorID,
			},
			"transactionInfo": map[string]any{
				"transactionId": numericTxID,
			},
			"meterValue": []map[string]any{
				{
					"timestamp": now.Format(time.RFC3339),
					"sampledValue": []map[string]any{
						{
							"value":     meterStopWh,
							"measurand": "Energy.Active.Import.Register",
							"unitOfMeasure": map[string]any{
								"unit": "Wh",
							},
						},
					},
				},
			},
			"stoppedReason": reason,
		}
	}

	return map[string]any{
		"transactionId": numericTxID,
		"meterStop":     meterStopWh,
		"timestamp":     now.Format(time.RFC3339),
		"reason":        reason,
	}
}

func numericTransactionID(txID string) int {
	if txID == "" {
		return 0
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(txID))
	return int(hasher.Sum32()%900000) + 100000
}
