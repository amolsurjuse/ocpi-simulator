# OCPP Simulator UI

Angular frontend for the OCPI/OCPP simulator.

## Local development

```bash
cd /Users/amolsurjuse/development/projects/ocpi-simulator/ui
npm install
npm start
```

Default UI URL:

- `http://localhost:4200`

When running the backend locally (`http://localhost:8081`), keep **API base URL** as:

- `http://localhost:8081`

## Environment sync

The Fleet screen includes **Environment sync**:

- enter `environmentUrl`
- set `getChargersPath` (default `/api/v1/chargers`)
- optionally provide bearer token
- import chargers/connectors into simulator fleet

Imported chargers can then be connected and operated from the same UI, with live event stream visible in Dashboard/Events.
