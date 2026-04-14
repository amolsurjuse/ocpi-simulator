export type OcppVersion = 'OCPP16J' | 'OCPP201';

export type ConnectionState = 'CONNECTED' | 'CONNECTING' | 'DISCONNECTED' | 'DISCONNECTING' | 'ERROR';

export type ConnectorStatus =
  | 'Available'
  | 'Preparing'
  | 'Charging'
  | 'SuspendedEV'
  | 'SuspendedEVSE'
  | 'Finishing'
  | 'Unavailable'
  | 'Faulted';

export interface TlsConfig {
  enabled: boolean;
  skipVerify?: boolean;
}

export interface TransportConfig {
  role: 'CP' | 'CSMS';
  csmsUrl: string;
  tls?: TlsConfig;
}

export interface ChargerConnector {
  connectorId: number;
  type: string;
  maxKw: number;
  status?: ConnectorStatus;
  errorCode?: string;
}

export interface ChargerSocConfig {
  enabled: boolean;
  startPercent: number;
  endPercent: number;
  ratePercentPerMin: number;
}

export interface ChargerMeteringConfig {
  energyWhStart: number;
  powerW: number;
  voltageV: number;
  currentA: number;
}

export interface ChargerClockConfig {
  timeZone: string;
  driftMsPerMin: number;
}

export interface ChargerBootConfig {
  vendor: string;
  model: string;
  firmwareVersion?: string;
}

export interface ChargerConfig {
  heartbeatIntervalSec: number;
  meterValuesIntervalSec?: number;
  soc?: ChargerSocConfig;
  metering?: ChargerMeteringConfig;
  clock?: ChargerClockConfig;
  boot?: ChargerBootConfig;
}

export interface ChargerCreateRequest {
  chargerId: string;
  ocppIdentity?: string;
  ocppVersion: OcppVersion;
  transport: TransportConfig;
  connectors?: ChargerConnector[];
  config?: ChargerConfig;
  tags?: Record<string, string>;
}

export interface ChargerCreateResponse {
  chargerId: string;
  status: string;
  links?: {
    self: string;
    connect?: string;
  };
}

export interface ChargerListItem {
  chargerId: string;
  ocppIdentity?: string;
  ocppVersion: OcppVersion;
  connectionState: ConnectionState;
  activeTransactions?: number;
}

export interface ChargerListResponse {
  items: ChargerListItem[];
  nextCursor?: string | null;
}

export interface ChargerDetails {
  chargerId: string;
  ocppIdentity?: string;
  ocppVersion: OcppVersion;
  connectionState: ConnectionState;
  config?: ChargerConfig;
  runtime?: {
    lastHeartbeatAt?: string;
    lastMessageAt?: string;
    activeTransactions?: ActiveTransaction[];
  };
  transport?: TransportConfig;
  connectors?: ChargerConnector[];
  tags?: Record<string, string>;
}

export interface ActiveTransaction {
  transactionId: string;
  connectorId: number;
  status?: string;
  meterStartWh?: number;
  meterStopWh?: number;
  startedAt?: string;
  authorizationId?: string;
  idTag?: string;
}

export interface ConnectionDetails {
  chargerId: string;
  connectionState: ConnectionState;
  remote?: string;
  since?: string;
}

export interface StatsResponse {
  chargersTotal?: number;
  connected?: number;
  connecting?: number;
  disconnected?: number;
  msgRateInPerSec?: number;
  msgRateOutPerSec?: number;
}

export interface EnvironmentImportRequest {
  environmentUrl: string;
  getChargersPath?: string;
  bearerToken?: string;
  headers?: Record<string, string>;
  skipTlsVerify?: boolean;
}

export interface EnvironmentImportResponse {
  environmentUrl: string;
  getChargersUrl: string;
  imported: number;
  created: number;
  updated: number;
  failed: number;
  errors?: string[];
}

export interface TapRequest {
  method: string;
  idToken?: {
    type: string;
    value: string;
  };
  idTag?: string;
  purpose?: string;
}

export interface PncStartRequest {
  ev?: {
    vin?: string;
    iso15118?: {
      enabled?: boolean;
      emaid?: string;
      contractCertId?: string;
      pncIdToken?: {
        type?: string;
        value?: string;
      };
    };
  };
  transaction?: {
    requestStart?: boolean;
    targetPowerW?: number;
  };
}

export interface ChargingStartRequest {
  auth?: {
    idTag?: string;
    authorizationId?: string;
  };
  meterStartWh?: number;
  targetPowerW?: number;
  chargingProfile?: {
    enabled?: boolean;
  };
}

export interface ChargingStopRequest {
  transactionId: string;
  reason?: string;
  meterStopWh?: number;
}

export interface MeterValuesRequest {
  transactionId?: string;
  samples?: Array<{
    measurand: string;
    value: number | string;
    unit?: string;
  }>;
  timestamp?: string;
}

export interface StatusUpdateRequest {
  status: ConnectorStatus;
  errorCode?: string;
}

export interface FaultInjectRequest {
  type: string;
  errorCode?: string;
  durationSec?: number;
  disconnect?: boolean;
}

export interface HeartbeatIntervalRequest {
  heartbeatIntervalSec: number;
}

export interface OcppSendRequest {
  action: string;
  payload: Record<string, unknown>;
  awaitResponse?: boolean;
  timeoutMs?: number;
}
