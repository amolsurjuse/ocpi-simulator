package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"ocpi-simulator/internal/fleet"
)

const defaultGetChargersPath = "api/v1/chargers"

type importEnvironmentChargersRequest struct {
	EnvironmentURL  string            `json:"environmentUrl"`
	GetChargersPath string            `json:"getChargersPath"`
	BearerToken     string            `json:"bearerToken"`
	Headers         map[string]string `json:"headers"`
	SkipTLSVerify   bool              `json:"skipTlsVerify"`
}

type importEnvironmentChargersResponse struct {
	EnvironmentURL string   `json:"environmentUrl"`
	GetChargersURL string   `json:"getChargersUrl"`
	Imported       int      `json:"imported"`
	Created        int      `json:"created"`
	Updated        int      `json:"updated"`
	Failed         int      `json:"failed"`
	Errors         []string `json:"errors,omitempty"`
}

func (a *App) handleImportEnvironmentChargers(w http.ResponseWriter, r *http.Request) {
	var payload importEnvironmentChargersRequest
	if err := decodeJSON(w, r, &payload); err != nil {
		return
	}

	baseURL, err := normalizeEnvironmentURL(payload.EnvironmentURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	getChargersURL, err := resolveGetChargersURL(baseURL, payload.GetChargersPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	items, err := a.fetchEnvironmentChargers(r.Context(), getChargersURL, payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	response := importEnvironmentChargersResponse{
		EnvironmentURL: baseURL.String(),
		GetChargersURL: getChargersURL,
	}

	for _, item := range items {
		charger, mapErr := mapEnvironmentCharger(item, baseURL)
		if mapErr != nil {
			response.Failed++
			if len(response.Errors) < 20 {
				response.Errors = append(response.Errors, mapErr.Error())
			}
			continue
		}

		if _, exists := a.fleet.GetCharger(charger.ChargerID); exists {
			if _, updateErr := a.fleet.UpdateCharger(charger); updateErr != nil {
				response.Failed++
				if len(response.Errors) < 20 {
					response.Errors = append(response.Errors, fmt.Sprintf("%s: %v", charger.ChargerID, updateErr))
				}
				continue
			}
			response.Updated++
			response.Imported++
			a.emitFleetEvent(fleet.Event{
				Type:      "CHARGER_SYNCED",
				Timestamp: time.Now().UTC(),
				ChargerID: charger.ChargerID,
				Message:   "updated",
			})
			continue
		}

		if _, createErr := a.fleet.AddCharger(charger); createErr != nil {
			response.Failed++
			if len(response.Errors) < 20 {
				response.Errors = append(response.Errors, fmt.Sprintf("%s: %v", charger.ChargerID, createErr))
			}
			continue
		}
		response.Created++
		response.Imported++
		a.emitFleetEvent(fleet.Event{
			Type:      "CHARGER_SYNCED",
			Timestamp: time.Now().UTC(),
			ChargerID: charger.ChargerID,
			Message:   "created",
		})
	}

	a.emitFleetEvent(fleet.Event{
		Type:      "ENVIRONMENT_SYNC_COMPLETED",
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("imported=%d created=%d updated=%d failed=%d", response.Imported, response.Created, response.Updated, response.Failed),
		Data: map[string]any{
			"environmentUrl": response.EnvironmentURL,
			"getChargersUrl": response.GetChargersURL,
			"imported":       response.Imported,
			"created":        response.Created,
			"updated":        response.Updated,
			"failed":         response.Failed,
		},
	})

	respondJSON(w, http.StatusOK, response)
}

func (a *App) fetchEnvironmentChargers(ctx context.Context, getChargersURL string, payload importEnvironmentChargersRequest) ([]map[string]any, error) {
	reqCtx, cancel := context.WithTimeout(ctx, a.cfg.EnvironmentTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, getChargersURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if token := strings.TrimSpace(payload.BearerToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range payload.Headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	client := a.httpClient
	if payload.SkipTLSVerify {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		client = &http.Client{
			Timeout:   a.cfg.EnvironmentTimeout,
			Transport: transport,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getChargers request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read getChargers response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if len(message) > 300 {
			message = message[:300] + "..."
		}
		return nil, fmt.Errorf("getChargers returned %d: %s", resp.StatusCode, message)
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("getChargers returned invalid JSON: %w", err)
	}

	chargers := extractChargerCollection(decoded)
	if len(chargers) == 0 {
		return nil, fmt.Errorf("getChargers response did not include chargers")
	}
	return chargers, nil
}

func normalizeEnvironmentURL(rawURL string) (*url.URL, error) {
	normalized := strings.TrimSpace(rawURL)
	if normalized == "" {
		return nil, fmt.Errorf("environmentUrl is required")
	}
	if !strings.Contains(normalized, "://") {
		normalized = "https://" + normalized
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("invalid environmentUrl")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("environmentUrl must include scheme and host")
	}
	return parsed, nil
}

func resolveGetChargersURL(environmentURL *url.URL, getChargersPath string) (string, error) {
	resolvedPath := strings.TrimSpace(getChargersPath)
	if resolvedPath != "" {
		if absolute, err := url.Parse(resolvedPath); err == nil && absolute.Scheme != "" && absolute.Host != "" {
			return absolute.String(), nil
		}
	}

	if resolvedPath == "" {
		basePath := strings.Trim(environmentURL.Path, "/")
		if basePath == "" {
			resolvedPath = "/" + defaultGetChargersPath
		} else {
			resolvedPath = "/" + basePath + "/" + defaultGetChargersPath
		}
	}

	joined := *environmentURL
	if strings.HasPrefix(resolvedPath, "/") {
		joined.Path = path.Clean(resolvedPath)
	} else {
		basePath := strings.TrimSuffix(environmentURL.Path, "/")
		joined.Path = path.Clean(basePath + "/" + resolvedPath)
	}
	joined.RawQuery = ""
	joined.Fragment = ""

	if joined.Path == "." {
		joined.Path = "/"
	}
	return joined.String(), nil
}

func extractChargerCollection(payload any) []map[string]any {
	if arr := asMapSlice(payload); len(arr) > 0 {
		return arr
	}

	bestScore := 0
	var best []map[string]any

	var walk func(node any)
	walk = func(node any) {
		if candidate := asMapSlice(node); len(candidate) > 0 {
			score := scoreChargerArray(candidate)
			if score > bestScore || (score == bestScore && len(candidate) > len(best)) {
				bestScore = score
				best = candidate
			}
		}

		switch typed := node.(type) {
		case map[string]any:
			for _, value := range typed {
				walk(value)
			}
		case []any:
			for _, value := range typed {
				walk(value)
			}
		}
	}

	walk(payload)
	if bestScore > 0 {
		return best
	}
	return nil
}

func scoreChargerArray(items []map[string]any) int {
	score := 0
	limit := len(items)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		item := items[i]
		if firstString(item, "chargerId", "chargerID", "id", "cpId", "chargePointId") != "" {
			score += 3
		}
		if _, ok := item["connectors"]; ok {
			score += 2
		}
		if _, ok := item["evses"]; ok {
			score += 1
		}
	}
	return score
}

func mapEnvironmentCharger(raw map[string]any, environmentURL *url.URL) (fleet.Charger, error) {
	chargerID := firstString(raw, "chargerId", "chargerID", "id", "cpId", "chargePointId")
	if chargerID == "" {
		return fleet.Charger{}, fmt.Errorf("missing chargerId in environment payload")
	}

	ocppIdentity := firstString(raw, "ocppIdentity", "ocpp_id", "ocppId", "identity", "chargePointId")
	if ocppIdentity == "" {
		ocppIdentity = chargerID
	}

	ocppVersion := normalizeOCPPVersion(firstString(raw, "ocppVersion", "version", "protocol"))
	connectionState := normalizeConnectionState(firstString(raw, "connectionState", "connectionStatus", "state"))

	transport := fleet.Transport{
		Role: "CP",
		TLS:  fleet.TLS{Enabled: strings.EqualFold(environmentURL.Scheme, "https"), SkipVerify: false},
	}
	if transportMap := asMap(raw["transport"]); transportMap != nil {
		if role := firstString(transportMap, "role"); role != "" {
			transport.Role = role
		}
		if csmsURL := firstString(transportMap, "csmsUrl", "endpoint", "wsUrl"); csmsURL != "" {
			transport.CSMSURL = csmsURL
		}
		if tlsMap := asMap(transportMap["tls"]); tlsMap != nil {
			if enabled, ok := toBool(tlsMap["enabled"]); ok {
				transport.TLS.Enabled = enabled
			}
			if skipVerify, ok := toBool(tlsMap["skipVerify"]); ok {
				transport.TLS.SkipVerify = skipVerify
			}
		}
	}
	if transport.CSMSURL == "" {
		transport.CSMSURL = firstString(raw, "csmsUrl", "ocppUrl", "wsUrl", "endpoint")
	}
	if transport.CSMSURL == "" {
		transport.CSMSURL = deriveEnvironmentCSMSURL(environmentURL, ocppVersion, ocppIdentity)
	}

	charger := fleet.Charger{
		ChargerID:       chargerID,
		OCPPIdentity:    ocppIdentity,
		OCPPVersion:     ocppVersion,
		ConnectionState: connectionState,
		Transport:       transport,
		Connectors:      extractEnvironmentConnectors(raw),
		Tags:            extractEnvironmentTags(raw),
	}
	return charger, nil
}

func normalizeOCPPVersion(version string) string {
	switch strings.ToUpper(strings.TrimSpace(version)) {
	case "2.0.1", "OCPP201", "OCPP20", "OCPP2.0.1":
		return "OCPP201"
	case "1.6", "OCPP16", "OCPP1.6", "OCPP16J":
		return "OCPP16J"
	default:
		return "OCPP16J"
	}
}

func normalizeConnectionState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "CONNECTED", "ONLINE":
		return "CONNECTED"
	case "CONNECTING":
		return "CONNECTING"
	case "DISCONNECTING":
		return "DISCONNECTING"
	case "ERROR", "FAILED":
		return "ERROR"
	default:
		return "DISCONNECTED"
	}
}

func deriveEnvironmentCSMSURL(environmentURL *url.URL, ocppVersion string, identity string) string {
	if identity == "" {
		identity = "sim-unknown"
	}
	scheme := "ws"
	if strings.EqualFold(environmentURL.Scheme, "https") {
		scheme = "wss"
	}
	protocol := "1.6"
	if strings.EqualFold(ocppVersion, "OCPP201") {
		protocol = "2.0.1"
	}
	return fmt.Sprintf("%s://%s/ocpp/%s/%s", scheme, environmentURL.Host, protocol, url.PathEscape(identity))
}

func extractEnvironmentConnectors(raw map[string]any) []fleet.Connector {
	connectors := []fleet.Connector{}
	seenConnectorIDs := map[int]bool{}
	nextConnectorID := 1

	appendConnector := func(candidate fleet.Connector) {
		if candidate.ConnectorID <= 0 || seenConnectorIDs[candidate.ConnectorID] {
			for seenConnectorIDs[nextConnectorID] {
				nextConnectorID++
			}
			candidate.ConnectorID = nextConnectorID
			nextConnectorID++
		}
		seenConnectorIDs[candidate.ConnectorID] = true
		if candidate.Type == "" {
			candidate.Type = "CCS"
		}
		if candidate.MaxKw <= 0 {
			candidate.MaxKw = 50
		}
		if candidate.Status == "" {
			candidate.Status = "Available"
		}
		if candidate.ErrorCode == "" {
			candidate.ErrorCode = "NoError"
		}
		connectors = append(connectors, candidate)
	}

	for _, rawConnector := range asMapSlice(raw["connectors"]) {
		appendConnector(mapEnvironmentConnector(rawConnector))
	}

	for _, evse := range asMapSlice(raw["evses"]) {
		for _, evseConnector := range asMapSlice(evse["connectors"]) {
			appendConnector(mapEnvironmentConnector(evseConnector))
		}
	}

	return connectors
}

func mapEnvironmentConnector(raw map[string]any) fleet.Connector {
	connector := fleet.Connector{
		ConnectorID: toInt(firstNonNil(raw, "connectorId", "connectorID", "id", "connectorNo")),
		Type:        firstString(raw, "type", "standard", "format"),
		MaxKw:       toFloat(firstNonNil(raw, "maxKw", "maxPowerKw", "powerKw", "maxPower")),
		Status:      firstString(raw, "status", "state"),
		ErrorCode:   firstString(raw, "errorCode", "error"),
		FaultType:   firstString(raw, "faultType", "fault"),
	}

	if connector.MaxKw <= 0 {
		powerW := toFloat(firstNonNil(raw, "powerW", "maxPowerW"))
		if powerW > 0 {
			connector.MaxKw = powerW / 1000.0
		}
	}
	return connector
}

func extractEnvironmentTags(raw map[string]any) map[string]string {
	tags := map[string]string{}

	if source := asMap(raw["tags"]); source != nil {
		for key, value := range source {
			if str := strings.TrimSpace(fmt.Sprintf("%v", value)); str != "" {
				tags[key] = str
			}
		}
	}

	for _, key := range []string{"siteId", "locationId", "tenantId", "groupId"} {
		if value := firstString(raw, key); value != "" {
			tags[strings.TrimSuffix(strings.ToLower(key), "id")] = value
		}
	}
	return tags
}

func asMap(value any) map[string]any {
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return mapped
}

func asMapSlice(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		mapped, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, mapped)
	}
	return result
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprintf("%v", data[key]))
		if value == "" || value == "<nil>" {
			continue
		}
		return value
	}
	return ""
}

func firstNonNil(data map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := data[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func toFloat(value any) float64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		return 0
	}
}

func toInt(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func toBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	}
	return false, false
}
