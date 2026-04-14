const DEV_SIMULATOR_BASE_URL = 'http://localhost:8081';
const DEV_OCPP_WS_ORIGIN = 'ws://localhost:8081';

type LocationLike = Pick<Location, 'hostname' | 'origin'> | null | undefined;

export function resolveDefaultApiBaseUrl(locationLike: LocationLike = globalThis?.location): string {
  if (!locationLike || !locationLike.origin || locationLike.origin === 'null') {
    return DEV_SIMULATOR_BASE_URL;
  }

  if (isLocalHost(locationLike.hostname)) {
    return DEV_SIMULATOR_BASE_URL;
  }

  return locationLike.origin;
}

export function buildDefaultCsmsUrl(
  ocppVersion: string | null | undefined,
  identity: string,
  chargerId: string,
  locationLike: LocationLike = globalThis?.location
) {
  const protocol = ocppVersion === 'OCPP201' ? '2.0.1' : '1.6';
  const chargePointId = encodeURIComponent((identity || chargerId || 'sim-000001').trim());
  return `${resolveDefaultWsOrigin(locationLike)}/ocpp/${protocol}/${chargePointId}`;
}

function isLocalHost(hostname: string | null | undefined) {
  return hostname === 'localhost' || hostname === '127.0.0.1';
}

function resolveDefaultWsOrigin(locationLike: LocationLike) {
  if (!locationLike?.origin || locationLike.origin === 'null') {
    return DEV_OCPP_WS_ORIGIN;
  }
  if (isLocalHost(locationLike.hostname)) {
    return DEV_OCPP_WS_ORIGIN;
  }
  if (locationLike.origin.startsWith('https://')) {
    return locationLike.origin.replace(/^https:\/\//, 'wss://');
  }
  if (locationLike.origin.startsWith('http://')) {
    return locationLike.origin.replace(/^http:\/\//, 'ws://');
  }
  return DEV_OCPP_WS_ORIGIN;
}
