# OCPI Simulator

Production-grade OCPI 2.2.1 mock service for CPO + EMSP flows with WebSocket event streaming.

## Features
- OCPI 2.2.1 versions + endpoints discovery
- Credentials, Locations, Tariffs, Sessions, CDRs, Commands
- In-memory charger/session store
- WebSocket event stream for DMS, start, meter_value, stop, unplug
- REST API to add/remove chargers and drive sessions
- Docker and Kubernetes manifests

## Quick start

```bash
go run ./cmd/ocpi-simulator
```

Default port: `8081`

Base URL defaults to `http://localhost:8081`

## Environment variables
- `PORT` (default `8081`)
- `BASE_URL` (default `http://localhost:<PORT>`)
- `EVENT_INTERVAL` (default `5s`)
- `READ_TIMEOUT` (default `10s`)
- `WRITE_TIMEOUT` (default `20s`)
- `SHUTDOWN_TIMEOUT` (default `10s`)
- `LOG_LEVEL` (`debug`, `info`, `warn`, `error`)

## WebSocket

- `GET /ws`
- Broadcast events:
  - `dms`
  - `boot_notification`
  - `start`
  - `meter_value`
  - `stop`
  - `unplug`

Example event payload:
```json
{
  "type": "meter_value",
  "timestamp": "2026-02-09T01:23:45Z",
  "charger_id": "chg-1234",
  "location_id": "loc-chg-1234",
  "evse_uid": "EVSE-chg-1234",
  "session_id": "ses-aaaa",
  "kwh": 1.4,
  "meter_value": 1.4
}
```

## Charger management API

Legacy endpoints (kept for compatibility):
- `GET /api/chargers`
- `POST /api/chargers`
- `GET /api/chargers/{chargerID}`
- `DELETE /api/chargers/{chargerID}`
- `POST /api/chargers/{chargerID}/sessions` (start session)
- `POST /api/sessions/{sessionID}/stop`
- `POST /api/sessions/{sessionID}/meter`

Fleet management API (v1):
- `POST /api/v1/chargers`
- `DELETE /api/v1/chargers/{chargerId}?force=true`
- `GET /api/v1/chargers?status=CONNECTED&ocppVersion=OCPP201&limit=100&cursor=...`
- `GET /api/v1/chargers/{chargerId}`
- `PATCH /api/v1/chargers/{chargerId}/config`
- `POST /api/v1/chargers/{chargerId}/connection/connect`
- `POST /api/v1/chargers/{chargerId}/connection/disconnect`
- `GET /api/v1/chargers/{chargerId}/connection`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/tap`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/plug-and-charge/start`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/plug-and-charge/stop`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/charging/start`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/charging/stop`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/meter-values/send`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/status`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/faults/inject`
- `POST /api/v1/chargers/{chargerId}/connectors/{connectorId}/faults/clear`
- `POST /api/v1/chargers/{chargerId}/heartbeat/send`
- `POST /api/v1/chargers/{chargerId}/heartbeat/interval`
- `POST /api/v1/chargers/{chargerId}/ocpp/send`
- `POST /api/v1/chargers/bulk`
- `POST /api/v1/chargers/bulk/connect`
- `POST /api/v1/chargers/bulk/disconnect`
- `GET /api/v1/stats`
- `GET /api/v1/events/stream?chargerId=sim-000001` (SSE)

Create charger:
```json
{
  "chargerId": "sim-000001",
  "ocppIdentity": "CP_000001",
  "ocppVersion": "OCPP16J",
  "transport": {
    "role": "CP",
    "csmsUrl": "wss://csms.example.com/ocpp",
    "tls": { "enabled": true, "skipVerify": false }
  },
  "connectors": [
    { "connectorId": 1, "type": "CCS", "maxKw": 150 },
    { "connectorId": 2, "type": "TYPE2", "maxKw": 22 }
  ],
  "config": {
    "heartbeatIntervalSec": 60,
    "meterValuesIntervalSec": 15,
    "soc": { "enabled": true, "startPercent": 35, "endPercent": 80, "ratePercentPerMin": 1.2 },
    "metering": { "energyWhStart": 1200000, "powerW": 11000, "voltageV": 400, "currentA": 16 },
    "clock": { "timeZone": "UTC", "driftMsPerMin": 0 },
    "boot": { "vendor": "SimVendor", "model": "SimModel-1", "firmwareVersion": "1.0.0" }
  },
  "tags": { "site": "lab", "shard": "pod-3" }
}
```

## OCPP WebSocket

Connect a charge point using OCPP 1.6 or 2.0.1:

- `ws://<host>:8081/ocpp/1.6/{chargePointId}`
- `ws://<host>:8081/ocpp/2.0.1/{chargePointId}`

Supported action: `BootNotification` (returns `Accepted`).

## OCPI endpoints

- `GET /ocpi/versions`
- `GET /ocpi/2.2.1`
- `GET|POST|PUT /ocpi/2.2.1/credentials`
- `GET /ocpi/2.2.1/locations`
- `GET /ocpi/2.2.1/locations/{location_id}`
- `GET /ocpi/2.2.1/locations/{location_id}/{evse_uid}`
- `GET /ocpi/2.2.1/locations/{location_id}/{evse_uid}/{connector_id}`
- `GET /ocpi/2.2.1/tariffs`
- `GET /ocpi/2.2.1/sessions`
- `POST /ocpi/2.2.1/sessions`
- `PATCH /ocpi/2.2.1/sessions/{session_id}`
- `GET /ocpi/2.2.1/cdrs`
- `POST /ocpi/2.2.1/cdrs`
- `POST /ocpi/2.2.1/commands/{command}`

## Docker

```bash
docker build -t ocpi-simulator .
docker run -p 8081:8081 ocpi-simulator
```

## Kubernetes

```bash
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
```
