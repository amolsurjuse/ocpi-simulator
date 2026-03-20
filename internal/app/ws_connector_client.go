package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ocpi-simulator/internal/fleet"
)

type wsConnectorClient struct {
	baseURL string
	http    *http.Client
}

type wsConnectRequest struct {
	ChargerID   string `json:"chargerId"`
	URL         string `json:"url"`
	OcppVersion string `json:"ocppVersion"`
	SkipVerify  bool   `json:"skipVerify"`
	Boot        struct {
		Vendor string `json:"vendor"`
		Model  string `json:"model"`
	} `json:"boot"`
}

type wsDisconnectRequest struct {
	ChargerID string `json:"chargerId"`
	Reason    string `json:"reason"`
}

type wsConnectionInfo struct {
	ChargerID     string `json:"chargerId"`
	State         string `json:"state"`
	LastMessageAt string `json:"lastMessageAt,omitempty"`
	Error         string `json:"error,omitempty"`
}

type wsSendRequest struct {
	ChargerID string         `json:"chargerId"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload"`
}

func newWSConnectorClient(baseURL string) *wsConnectorClient {
	return &wsConnectorClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *wsConnectorClient) Connect(ctx context.Context, charger fleet.Charger, wsURL string) error {
	reqBody := wsConnectRequest{
		ChargerID:   charger.ChargerID,
		URL:         wsURL,
		OcppVersion: charger.OCPPVersion,
		SkipVerify:  charger.Transport.TLS.SkipVerify,
	}
	reqBody.Boot.Vendor = charger.Config.Boot.Vendor
	reqBody.Boot.Model = charger.Config.Boot.Model

	return c.doJSON(ctx, http.MethodPost, "/connect", reqBody, nil)
}

func (c *wsConnectorClient) Disconnect(ctx context.Context, chargerID, reason string) error {
	reqBody := wsDisconnectRequest{ChargerID: chargerID, Reason: reason}
	return c.doJSON(ctx, http.MethodPost, "/disconnect", reqBody, nil)
}

func (c *wsConnectorClient) Connection(ctx context.Context, chargerID string) (wsConnectionInfo, error) {
	var out wsConnectionInfo
	err := c.doJSON(ctx, http.MethodGet, "/connections/"+chargerID, nil, &out)
	return out, err
}

func (c *wsConnectorClient) LogEvent(ctx context.Context, event fleet.Event) error {
	return c.doJSON(ctx, http.MethodPost, "/events", event, nil)
}

func (c *wsConnectorClient) Send(ctx context.Context, chargerID, action string, payload map[string]any) error {
	reqBody := wsSendRequest{
		ChargerID: chargerID,
		Action:    action,
		Payload:   payload,
	}
	return c.doJSON(ctx, http.MethodPost, "/send", reqBody, nil)
}

func (c *wsConnectorClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	if c.baseURL == "" {
		return fmt.Errorf("connector base url is empty")
	}
	url := c.baseURL + path

	var reqBodyBytes []byte
	var err error
	if body != nil {
		reqBodyBytes, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("connector call failed: %s %s returned %d", method, path, resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}

	return nil
}
