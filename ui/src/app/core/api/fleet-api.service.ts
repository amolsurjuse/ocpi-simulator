import { HttpClient, HttpParams } from '@angular/common/http';
import { inject, Injectable, signal } from '@angular/core';
import { API_BASE_URL } from '../tokens';
import {
  ChargerCreateRequest,
  ChargerCreateResponse,
  ChargerDetails,
  ChargerListResponse,
  ChargingStartRequest,
  ChargingStopRequest,
  ConnectionDetails,
  EnvironmentImportRequest,
  EnvironmentImportResponse,
  FaultInjectRequest,
  HeartbeatIntervalRequest,
  MeterValuesRequest,
  OcppSendRequest,
  PncStartRequest,
  StatsResponse,
  StatusUpdateRequest,
  TapRequest
} from './models';

@Injectable({ providedIn: 'root' })
export class FleetApiService {
  private readonly http = inject(HttpClient);
  private readonly defaultBaseUrl = inject(API_BASE_URL);
  readonly baseUrl = signal<string>(this.normalizeBaseUrl(this.defaultBaseUrl));

  setBaseUrl(next: string) {
    this.baseUrl.set(this.normalizeBaseUrl(next));
  }

  getStats() {
    return this.http.get<StatsResponse>(this.api('/stats'));
  }

  importEnvironmentChargers(payload: EnvironmentImportRequest) {
    return this.http.post<EnvironmentImportResponse>(this.api('/environment/chargers/import'), payload);
  }

  listChargers(params: {
    status?: string;
    ocppVersion?: string;
    limit?: number;
    cursor?: string;
  }) {
    let httpParams = new HttpParams();
    if (params.status) {
      httpParams = httpParams.set('status', params.status);
    }
    if (params.ocppVersion) {
      httpParams = httpParams.set('ocppVersion', params.ocppVersion);
    }
    if (params.limit) {
      httpParams = httpParams.set('limit', String(params.limit));
    }
    if (params.cursor) {
      httpParams = httpParams.set('cursor', params.cursor);
    }
    return this.http.get<ChargerListResponse>(this.api('/chargers'), { params: httpParams });
  }

  getCharger(chargerId: string) {
    return this.http.get<ChargerDetails>(this.api(`/chargers/${encodeURIComponent(chargerId)}`));
  }

  createCharger(payload: ChargerCreateRequest) {
    return this.http.post<ChargerCreateResponse>(this.api('/chargers'), payload);
  }

  deleteCharger(chargerId: string, force?: boolean) {
    const params = force ? new HttpParams().set('force', 'true') : undefined;
    return this.http.delete<{ chargerId: string; status: string }>(
      this.api(`/chargers/${encodeURIComponent(chargerId)}`),
      { params }
    );
  }

  patchConfig(chargerId: string, payload: Partial<ChargerCreateRequest['config']>) {
    return this.http.patch<{ chargerId: string; status: string }>(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/config`),
      payload
    );
  }

  connectCharger(chargerId: string, payload: Record<string, unknown>) {
    return this.http.post<{ chargerId: string; status: string }>(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connection/connect`),
      payload
    );
  }

  disconnectCharger(chargerId: string, payload: Record<string, unknown>) {
    return this.http.post<{ chargerId: string; status: string }>(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connection/disconnect`),
      payload
    );
  }

  getConnection(chargerId: string) {
    return this.http.get<ConnectionDetails>(this.api(`/chargers/${encodeURIComponent(chargerId)}/connection`));
  }

  tap(chargerId: string, connectorId: number, payload: TapRequest) {
    return this.http.post(this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/tap`), payload);
  }

  pncStart(chargerId: string, connectorId: number, payload: PncStartRequest) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/plug-and-charge/start`),
      payload
    );
  }

  pncStop(chargerId: string, connectorId: number, payload: Record<string, unknown>) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/plug-and-charge/stop`),
      payload
    );
  }

  startCharging(chargerId: string, connectorId: number, payload: ChargingStartRequest) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/charging/start`),
      payload
    );
  }

  stopCharging(chargerId: string, connectorId: number, payload: ChargingStopRequest) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/charging/stop`),
      payload
    );
  }

  sendMeterValues(chargerId: string, connectorId: number, payload: MeterValuesRequest) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/meter-values/send`),
      payload
    );
  }

  setStatus(chargerId: string, connectorId: number, payload: StatusUpdateRequest) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/status`),
      payload
    );
  }

  injectFault(chargerId: string, connectorId: number, payload: FaultInjectRequest) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/faults/inject`),
      payload
    );
  }

  clearFault(chargerId: string, connectorId: number, payload: Record<string, unknown>) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/connectors/${connectorId}/faults/clear`),
      payload
    );
  }

  sendHeartbeat(chargerId: string) {
    return this.http.post(this.api(`/chargers/${encodeURIComponent(chargerId)}/heartbeat/send`), {});
  }

  setHeartbeatInterval(chargerId: string, payload: HeartbeatIntervalRequest) {
    return this.http.post(
      this.api(`/chargers/${encodeURIComponent(chargerId)}/heartbeat/interval`),
      payload
    );
  }

  ocppSend(chargerId: string, payload: OcppSendRequest) {
    return this.http.post(this.api(`/chargers/${encodeURIComponent(chargerId)}/ocpp/send`), payload);
  }

  bulkAdd(payload: Record<string, unknown>) {
    return this.http.post(this.api('/chargers/bulk'), payload);
  }

  bulkConnect(payload: Record<string, unknown>) {
    return this.http.post(this.api('/chargers/bulk/connect'), payload);
  }

  bulkDisconnect(payload: Record<string, unknown>) {
    return this.http.post(this.api('/chargers/bulk/disconnect'), payload);
  }

  openEventsStream(chargerId?: string) {
    const url = new URL(this.api('/events/stream'));
    if (chargerId) {
      url.searchParams.set('chargerId', chargerId);
    }
    return new EventSource(url.toString());
  }

  private api(path: string) {
    return `${this.baseUrl()}/api/v1${path}`;
  }

  private normalizeBaseUrl(value: string) {
    return value.replace(/\/+$/, '');
  }
}
