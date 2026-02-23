package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"log/slog"

	"ocpi-simulator/internal/fleet"
	"ocpi-simulator/internal/ocpi"
	"ocpi-simulator/internal/store"
	"ocpi-simulator/internal/ws"
)

type App struct {
	cfg   Config
	store *store.Store
	fleet *fleet.Store
	hub   *ws.Hub
	fleetHub *fleet.EventHub
	log   *slog.Logger
	credMu      sync.RWMutex
	credentials ocpi.Credentials
}

type Event struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	ChargerID string    `json:"charger_id,omitempty"`
	LocationID string   `json:"location_id,omitempty"`
	EvseUID   string    `json:"evse_uid,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Kwh       float64   `json:"kwh,omitempty"`
	MeterValue float64  `json:"meter_value,omitempty"`
	Message   string    `json:"message,omitempty"`
}

func New(cfg Config, logger *slog.Logger) *App {
	credentials := ocpi.Credentials{
		Token:        "dev-token",
		URL:          cfg.BaseURL + "/ocpi/2.2.1",
		PartyID:      "AMY",
		CountryCode:  "US",
		Roles: []ocpi.Role{
			{
				Role:        "CPO",
				PartyID:     "AMY",
				CountryCode: "US",
				BusinessDetails: ocpi.BusinessDetails{Name: "Electra Hub"},
			},
			{
				Role:        "EMSP",
				PartyID:     "AMY",
				CountryCode: "US",
				BusinessDetails: ocpi.BusinessDetails{Name: "Electra Hub"},
			},
		},
		BusinessDetails: ocpi.BusinessDetails{Name: "Electra Hub"},
		LastUpdated:     time.Now().UTC(),
	}

	return &App{
		cfg:   cfg,
		store: store.NewStore(),
		fleet: fleet.NewStore(),
		hub:   ws.NewHub(),
		fleetHub: fleet.NewEventHub(),
		log:   logger,
		credentials: credentials,
	}
}

func (a *App) StartBackground(ctx context.Context) {
	go a.hub.Run(ctx)
	stop := make(chan struct{})
	go a.fleetHub.Run(stop)
	go a.runMeterLoop(ctx)

	go func() {
		<-ctx.Done()
		close(stop)
	}()
}

func (a *App) runMeterLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.EventInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessions := a.store.ListSessions()
			for _, session := range sessions {
				if session.Status != "ACTIVE" {
					continue
				}
				session.Kwh += 0.2
				a.store.UpdateSession(session)
				a.emitEvent(Event{
					Type:      "meter_value",
					Timestamp: time.Now().UTC(),
					ChargerID: session.ChargerID,
					LocationID: session.LocationID,
					EvseUID:   session.EvseUID,
					SessionID: session.ID,
					Kwh:       session.Kwh,
					MeterValue: session.Kwh,
				})
			}
		}
	}
}

func (a *App) emitEvent(event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	a.hub.Broadcast(payload)
}

func (a *App) handleWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, rw, err := ws.Upgrade(w, r)
	if err != nil {
		a.log.Warn("websocket upgrade failed", "error", err)
		return
	}

	client := ws.NewClient(conn, rw)
	client.Run(a.hub)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (a *App) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		a.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"bytes", ww.bytes,
			"duration", time.Since(start).String(),
		)
	})
}

func ocpiResponse[T any](data T) ocpi.Response[T] {
	return ocpi.Response[T]{
		StatusCode: 1000,
		Timestamp:  time.Now().UTC(),
		Data:       data,
	}
}

func ocpiError(message string) ocpi.EmptyResponse {
	return ocpi.EmptyResponse{
		StatusCode:    2000,
		StatusMessage: message,
		Timestamp:     time.Now().UTC(),
	}
}

func normalizeCommand(command string) string {
	return strings.ToUpper(strings.TrimSpace(command))
}
