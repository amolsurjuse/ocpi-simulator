package app

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"ocpi-simulator/internal/fleet"
)

func (a *App) seedConfiguredChargers() {
	if !a.cfg.AutoSeedChargers {
		return
	}
	for _, chargerID := range a.cfg.AutoChargerIDs {
		chargerID = strings.TrimSpace(chargerID)
		if chargerID == "" {
			continue
		}
		if _, ok := a.fleet.GetCharger(chargerID); ok {
			continue
		}
		charger := a.defaultElectraHubCharger(chargerID)
		if _, err := a.fleet.AddCharger(charger); err != nil {
			a.log.Warn("auto seed charger failed", "chargerId", chargerID, "error", err)
			continue
		}
		a.emitFleetEvent(fleet.Event{
			Type:      "CHARGER_CREATED",
			Timestamp: time.Now().UTC(),
			ChargerID: chargerID,
			Message:   "auto_seeded",
		})
	}
}

func (a *App) runAutoConnectLoop(ctx context.Context) {
	if !a.cfg.AutoConnectChargers {
		return
	}
	a.reconcileChargerConnections()

	interval := a.cfg.AutoConnectInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.reconcileChargerConnections()
		}
	}
}

func (a *App) reconcileChargerConnections() {
	for _, charger := range a.fleet.ListChargers() {
		if charger.ConnectionState == "CONNECTED" || charger.ConnectionState == "CONNECTING" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			info, err := a.wsConnector.Connection(ctx, charger.ChargerID)
			cancel()
			if err == nil && (info.State == "CONNECTED" || info.State == "CONNECTING") {
				continue
			}
			at := time.Now().UTC()
			_, _ = a.fleet.UpdateConnectionState(charger.ChargerID, "DISCONNECTED", "", &at)
		}
		if err := a.connectChargerThroughConnector(charger.ChargerID, ""); err != nil {
			a.log.Warn("auto connect charger failed", "chargerId", charger.ChargerID, "error", err)
		}
	}
}

func (a *App) defaultElectraHubCharger(chargerID string) fleet.Charger {
	return fleet.Charger{
		ChargerID:    chargerID,
		OCPPIdentity: chargerID,
		OCPPVersion:  "OCPP16J",
		Transport: fleet.Transport{
			Role:    "CP",
			CSMSURL: a.ocppEndpointForIdentity(chargerID),
			TLS: fleet.TLS{
				SkipVerify: true,
			},
		},
		Connectors: []fleet.Connector{
			{ConnectorID: 1, Type: "CCS-2", MaxKw: 150, Status: "Available", ErrorCode: "NoError"},
		},
		Config: fleet.ChargerConfig{
			HeartbeatIntervalSec:   60,
			MeterValuesIntervalSec: 15,
			Metering: fleet.MeteringConfig{
				EnergyWhStart: 1200000,
				PowerW:        7200,
				VoltageV:      400,
				CurrentA:      18,
			},
			Clock: fleet.ClockConfig{
				TimeZone: "UTC",
			},
			Boot: fleet.BootConfig{
				Vendor:          "ElectraHub",
				Model:           "Simulator DC Fast",
				FirmwareVersion: "dev-auto",
			},
		},
		Tags: map[string]string{
			"fleet":  "electrahub-dev",
			"source": "auto-seed",
		},
	}
}

func (a *App) ocppEndpointForIdentity(identity string) string {
	base := strings.TrimRight(a.cfg.DefaultCSMSURL, "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, "/"+identity) {
		return base
	}
	return base + "/" + url.PathEscape(identity)
}

func (a *App) resolveCSMSURL(charger fleet.Charger, override string) string {
	targetURL := strings.TrimSpace(override)
	if targetURL == "" {
		targetURL = strings.TrimSpace(charger.Transport.CSMSURL)
	}
	if targetURL == "" || strings.Contains(targetURL, "example.com") {
		identity := charger.OCPPIdentity
		if identity == "" {
			identity = charger.ChargerID
		}
		targetURL = a.ocppEndpointForIdentity(identity)
	}
	if targetURL == "" {
		targetURL = a.defaultSimulatorOCPPURL(charger)
	}
	return targetURL
}

func (a *App) connectChargerThroughConnector(chargerID, csmsURL string) error {
	charger, ok := a.fleet.GetCharger(chargerID)
	if !ok {
		return errors.New("charger not found")
	}
	if charger.ConnectionState == "CONNECTED" || charger.ConnectionState == "CONNECTING" {
		return nil
	}

	targetURL := a.resolveCSMSURL(charger, csmsURL)
	if targetURL == "" {
		return errors.New("csms url is empty")
	}
	charger.Transport.CSMSURL = targetURL
	_, _ = a.fleet.UpdateCharger(charger)

	now := time.Now().UTC()
	_, _ = a.fleet.UpdateConnectionState(chargerID, "CONNECTING", hostFromURL(targetURL), &now)
	a.emitFleetEvent(fleet.Event{Type: "CONNECTING", Timestamp: now, ChargerID: chargerID})

	go func(charger fleet.Charger, targetURL string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if info, err := a.wsConnector.Connection(ctx, chargerID); err == nil {
			switch info.State {
			case "CONNECTED":
				connectedAt := time.Now().UTC()
				_, _ = a.fleet.UpdateConnectionState(chargerID, "CONNECTED", hostFromURL(targetURL), &connectedAt)
				_, _ = a.fleet.UpdateRuntime(chargerID, func(runtime *fleet.Runtime) {
					runtime.LastHeartbeatAt = &connectedAt
					runtime.LastMessageAt = &connectedAt
				})
				a.emitFleetEvent(fleet.Event{Type: "CONNECTED", Timestamp: connectedAt, ChargerID: chargerID})
				a.resetEventStep(chargerID)
				return
			case "CONNECTING":
				return
			default:
				_ = a.wsConnector.Disconnect(ctx, chargerID, "RECONNECT")
			}
		}

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
	}(charger, targetURL)

	return nil
}
