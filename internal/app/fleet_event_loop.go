package app

import (
	"context"
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
		if charger.ConnectionState != "CONNECTED" {
			continue
		}
		if len(charger.Connectors) == 0 {
			continue
		}

		connectorID := charger.Connectors[0].ConnectorID
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
		case 1:
			txID := a.startSimTransaction(charger, connectorID, now)
			a.emitFleetEvent(fleet.Event{
				Type:          "START",
				Timestamp:     now,
				ChargerID:     charger.ChargerID,
				ConnectorID:   connectorID,
				TransactionID: txID,
			})
		case 2:
			txID, meterWh := a.sendSimMeterValue(charger, now)
			if txID != "" {
				a.emitFleetEvent(fleet.Event{
					Type:          "METER_VALUE",
					Timestamp:     now,
					ChargerID:     charger.ChargerID,
					ConnectorID:   connectorID,
					TransactionID: txID,
					Data: map[string]any{
						"meterWh": meterWh,
					},
				})
			}
		case 3:
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
			}
		case 4:
			a.emitFleetEvent(fleet.Event{
				Type:        "UNPLUG",
				Timestamp:   now,
				ChargerID:   charger.ChargerID,
				ConnectorID: connectorID,
				Message:     "ev_disconnected",
			})
		}
	}
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
