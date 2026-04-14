import { CommonModule } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnDestroy, OnInit, computed, inject, signal } from '@angular/core';
import { FormArray, FormBuilder, FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { merge, Observable } from 'rxjs';
import { FleetApiService } from './core/api/fleet-api.service';
import {
  ActiveTransaction,
  ChargerConnector,
  ChargerCreateRequest,
  ChargerDetails,
  ChargerListItem,
  ConnectionDetails,
  ConnectorStatus,
  EnvironmentImportResponse,
  OcppVersion,
  StatsResponse
} from './core/api/models';
import { buildDefaultCsmsUrl } from './core/dev-env';

type ScreenKey = 'dashboard' | 'fleet' | 'provision' | 'operations' | 'events';
type ConfirmationAction = 'startCharging' | 'stopCharging';

@Component({
  selector: 'app-root',
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './app.component.html',
  styleUrl: './app.component.css'
})
export class AppComponent implements OnInit, OnDestroy {
  private readonly api = inject(FleetApiService);
  private readonly fb = inject(FormBuilder);
  private readonly destroyRef = inject(DestroyRef);
  private eventSource: EventSource | null = null;
  private lastSuggestedCreateCsmsUrl = buildDefaultCsmsUrl('OCPP16J', 'CP_000001', 'sim-000001');

  readonly baseUrlControl = new FormControl(this.api.baseUrl(), {
    nonNullable: true,
    validators: [Validators.required]
  });
  readonly environmentUrlControl = new FormControl(this.api.baseUrl(), {
    nonNullable: true,
    validators: [Validators.required]
  });
  readonly getChargersPathControl = new FormControl('/api/v1/chargers', {
    nonNullable: true,
    validators: [Validators.required]
  });
  readonly bearerTokenControl = new FormControl('', { nonNullable: true });
  readonly skipTlsVerifyControl = new FormControl(false, { nonNullable: true });

  readonly stats = signal<StatsResponse | null>(null);
  readonly chargers = signal<ChargerListItem[]>([]);
  readonly environmentSyncSummary = signal<EnvironmentImportResponse | null>(null);
  readonly selectedCharger = signal<ChargerDetails | null>(null);
  readonly selectedConnection = signal<ConnectionDetails | null>(null);
  readonly selectedChargerId = computed(() => this.selectedCharger()?.chargerId ?? '');
  readonly activeScreen = signal<ScreenKey>('dashboard');
  readonly events = signal<Array<{ at: string; type: string; payload: string }>>([]);
  readonly actionLog = signal<string[]>([]);
  readonly isLoading = signal(false);
  readonly error = signal<string | null>(null);
  readonly eventStreamConnected = signal(false);
  readonly pendingConfirmation = signal<{ action: ConfirmationAction; title: string; message: string } | null>(null);
  readonly screens: Array<{ key: ScreenKey; label: string; description: string }> = [
    { key: 'dashboard', label: 'Dashboard', description: 'Fleet health and live status' },
    { key: 'fleet', label: 'Fleet', description: 'Inspect chargers and connector state' },
    { key: 'events', label: 'Events', description: 'Observe simulator event stream' }
  ];

  readonly listFilters = this.fb.group({
    status: [''],
    ocppVersion: [''],
    limit: [100],
    cursor: ['']
  });

  readonly createForm = this.fb.group({
    chargerId: ['sim-000001', Validators.required],
    ocppIdentity: ['CP_000001'],
    ocppVersion: ['OCPP16J', Validators.required],
    transport: this.fb.group({
      role: ['CP'],
      csmsUrl: [this.lastSuggestedCreateCsmsUrl, Validators.required],
      tls: this.fb.group({
        enabled: [true],
        skipVerify: [false]
      })
    }),
    connectors: this.fb.array([this.buildConnector()]),
    config: this.fb.group({
      heartbeatIntervalSec: [60, Validators.required],
      meterValuesIntervalSec: [15],
      soc: this.fb.group({
        enabled: [true],
        startPercent: [35],
        endPercent: [80],
        ratePercentPerMin: [1.2]
      }),
      metering: this.fb.group({
        energyWhStart: [1200000],
        powerW: [11000],
        voltageV: [400],
        currentA: [16]
      }),
      clock: this.fb.group({
        timeZone: ['UTC'],
        driftMsPerMin: [0]
      }),
      boot: this.fb.group({
        vendor: ['SimVendor'],
        model: ['SimModel-1'],
        firmwareVersion: ['1.0.0']
      })
    }),
    tags: this.fb.group({
      site: ['lab'],
      shard: ['pod-3']
    })
  });

  readonly configForm = this.fb.group({
    heartbeatIntervalSec: [60],
    meterValuesIntervalSec: [15],
    socStartPercent: [35],
    socEndPercent: [80]
  });

  readonly connectionForm = this.fb.group({
    csmsUrl: [this.lastSuggestedCreateCsmsUrl],
    reconnectEnabled: [true],
    minBackoffMs: [200],
    maxBackoffMs: [5000]
  });

  readonly disconnectForm = this.fb.group({
    reason: ['OPERATOR_REQUEST'],
    closeCode: [1000]
  });

  readonly tapForm = this.fb.group({
    connectorId: [1],
    method: ['RFID'],
    idTokenType: ['ISO14443'],
    idTokenValue: ['04AABBCCDDEE11'],
    idTag: ['ABC12345'],
    purpose: ['START_OR_STOP']
  });

  readonly pncStartForm = this.fb.group({
    connectorId: [1],
    vin: ['WVWZZZ1JZXW000001'],
    emaid: ['DE-ABC-1234567-8'],
    contractCertId: ['cert-001'],
    pncIdTokenValue: ['DE-ABC-1234567-8'],
    targetPowerW: [11000],
    requestStart: [true]
  });

  readonly pncStopForm = this.fb.group({
    connectorId: [1],
    reason: ['EV_DISCONNECTED'],
    requestStopTransaction: [true]
  });

  readonly chargingStartForm = this.fb.group({
    connectorId: [1],
    idTag: ['ABC12345'],
    authorizationId: ['auth-9f2c'],
    meterStartWh: [1200500],
    targetPowerW: [11000]
  });

  readonly chargingStopForm = this.fb.group({
    connectorId: [1],
    transactionId: ['tx-10001', Validators.required],
    reason: ['LOCAL'],
    meterStopWh: [1209500]
  });

  readonly meterForm = this.fb.group({
    connectorId: [1],
    transactionId: ['tx-10001'],
    energyWh: [1205500],
    powerW: [10800],
    socPercent: [44],
    timestamp: [new Date().toISOString()]
  });

  readonly statusForm = this.fb.group({
    connectorId: [1],
    status: ['Available'],
    errorCode: ['NoError']
  });

  readonly faultForm = this.fb.group({
    connectorId: [1],
    type: ['GROUND_FAILURE'],
    errorCode: ['GroundFailure'],
    durationSec: [120],
    disconnect: [false]
  });

  readonly heartbeatForm = this.fb.group({
    heartbeatIntervalSec: [20]
  });

  readonly ocppSendForm = this.fb.group({
    action: ['BootNotification'],
    payload: ['{ "chargePointVendor": "SimVendor", "chargePointModel": "SimModel-1" }'],
    awaitResponse: [true],
    timeoutMs: [5000]
  });

  get connectors() {
    return this.createForm.get('connectors') as FormArray;
  }

  ngOnInit() {
    this.syncCreateCsmsUrl();
    merge(
      this.createForm.controls.chargerId.valueChanges,
      this.createForm.controls.ocppIdentity.valueChanges,
      this.createForm.controls.ocppVersion.valueChanges
    )
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe(() => this.syncCreateCsmsUrl());

    this.fetchStats();
    this.listChargerFleet();
  }

  ngOnDestroy() {
    this.closeEventStream();
  }

  setScreen(screen: ScreenKey) {
    this.activeScreen.set(screen);
  }

  trackByIndex(index: number) {
    return index;
  }

  trackByChargerId(_: number, charger: ChargerListItem) {
    return charger.chargerId;
  }

  trackByConnectorId(_: number, connector: ChargerConnector) {
    return connector.connectorId;
  }

  applyBaseUrl() {
    if (this.baseUrlControl.invalid) {
      return;
    }
    const nextBaseUrl = this.baseUrlControl.value.trim();
    this.api.setBaseUrl(nextBaseUrl);
    if (!this.environmentUrlControl.value.trim()) {
      this.environmentUrlControl.setValue(nextBaseUrl);
    }
    this.closeEventStream();
    this.fetchStats();
    this.listChargerFleet();
    this.logAction('Base URL updated.');
  }

  importEnvironmentChargers() {
    if (this.environmentUrlControl.invalid || this.getChargersPathControl.invalid) {
      this.error.set('Environment URL and getChargers path are required.');
      return;
    }

    const environmentUrl = this.environmentUrlControl.value.trim();
    const getChargersPath = this.getChargersPathControl.value.trim();
    const bearerToken = this.bearerTokenControl.value.trim();

    this.runAction(
      'Environment chargers imported',
      this.api.importEnvironmentChargers({
        environmentUrl,
        getChargersPath,
        bearerToken: bearerToken || undefined,
        skipTlsVerify: this.skipTlsVerifyControl.value
      }),
      (summary) => {
        this.environmentSyncSummary.set(summary);
        this.listChargerFleet();
        this.fetchStats();
        this.logAction(
          `Import summary: imported ${summary.imported}, created ${summary.created}, updated ${summary.updated}, failed ${summary.failed}`
        );
      }
    );
  }

  fetchStats() {
    this.runAction('Stats refreshed', this.api.getStats(), (stats) => this.stats.set(stats));
  }

  listChargerFleet() {
    const { status, ocppVersion, limit, cursor } = this.listFilters.getRawValue();
    this.runAction(
      'Charger list refreshed',
      this.api.listChargers({
        status: status || undefined,
        ocppVersion: ocppVersion || undefined,
        limit: limit || undefined,
        cursor: cursor || undefined
      }),
      (response) => this.chargers.set(response.items)
    );
  }

  selectCharger(chargerId: string) {
    if (!chargerId) {
      return;
    }
    this.runAction('Charger loaded', this.api.getCharger(chargerId), (charger) => {
      this.selectedCharger.set(charger);
      this.seedConfigForm(charger);
      this.seedConnectionForm(charger);
      this.fetchConnection(chargerId);
    });
  }

  fetchConnection(chargerId?: string) {
    const id = chargerId ?? this.selectedChargerId();
    if (!id) {
      return;
    }
    this.runAction('Connection refreshed', this.api.getConnection(id), (connection) =>
      this.selectedConnection.set(connection)
    );
  }

  createCharger() {
    if (this.createForm.invalid) {
      this.error.set('Please fill required fields.');
      return;
    }
    const payload = this.buildCreatePayload();
    this.runAction('Charger created', this.api.createCharger(payload), () => {
      this.bumpDefaultChargerId();
      this.listChargerFleet();
      this.setScreen('fleet');
      this.selectCharger(payload.chargerId);
    });
  }

  deleteCharger(chargerId: string, force = false) {
    this.runAction('Charger deleted', this.api.deleteCharger(chargerId, force), () => {
      this.listChargerFleet();
      if (this.selectedChargerId() === chargerId) {
        this.selectedCharger.set(null);
      }
    });
  }

  patchConfig() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.configForm.getRawValue();
    const payload: ChargerCreateRequest['config'] = {
      heartbeatIntervalSec: this.toNumber(raw.heartbeatIntervalSec, 60),
      meterValuesIntervalSec: this.toNumber(raw.meterValuesIntervalSec, 15),
      soc: {
        enabled: true,
        startPercent: this.toNumber(raw.socStartPercent, 35),
        endPercent: this.toNumber(raw.socEndPercent, 80),
        ratePercentPerMin: 1
      }
    };
    this.runAction('Config updated', this.api.patchConfig(chargerId, payload), () => {
      this.selectCharger(chargerId);
    });
  }

  connectCharger() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.connectionForm.getRawValue();
    const payload = {
      csmsUrl: raw.csmsUrl,
      reconnectPolicy: {
        enabled: raw.reconnectEnabled,
        minBackoffMs: this.toNumber(raw.minBackoffMs, 200),
        maxBackoffMs: this.toNumber(raw.maxBackoffMs, 5000)
      }
    };
    this.runAction('Connect requested', this.api.connectCharger(chargerId, payload), () => {
      this.fetchConnection(chargerId);
      this.refreshSelectedCharger(chargerId);
    });
  }

  disconnectCharger() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.disconnectForm.getRawValue();
    const payload = {
      reason: raw.reason,
      closeCode: this.toNumber(raw.closeCode, 1000)
    };
    this.runAction('Disconnect requested', this.api.disconnectCharger(chargerId, payload), () => {
      this.fetchConnection(chargerId);
      this.refreshSelectedCharger(chargerId);
    });
  }

  tap() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.tapForm.getRawValue();
    const payload = {
      method: raw.method ?? 'RFID',
      idToken: {
        type: raw.idTokenType ?? '',
        value: raw.idTokenValue ?? ''
      },
      idTag: raw.idTag ?? '',
      purpose: raw.purpose ?? ''
    };
    this.runAction('Tap sent', this.api.tap(chargerId, Number(raw.connectorId), payload), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  startPnc() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.pncStartForm.getRawValue();
    const payload = {
      ev: {
        vin: raw.vin ?? '',
        iso15118: {
          enabled: true,
          emaid: raw.emaid ?? '',
          contractCertId: raw.contractCertId ?? '',
          pncIdToken: {
            type: 'ISO15118',
            value: raw.pncIdTokenValue ?? ''
          }
        }
      },
      transaction: {
        requestStart: raw.requestStart ?? true,
        targetPowerW: this.toNumber(raw.targetPowerW, 11000)
      }
    };
    this.runAction('PnC start queued', this.api.pncStart(chargerId, Number(raw.connectorId), payload), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  stopPnc() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.pncStopForm.getRawValue();
    const payload = {
      reason: raw.reason,
      requestStopTransaction: raw.requestStopTransaction
    };
    this.runAction('PnC stop queued', this.api.pncStop(chargerId, Number(raw.connectorId), payload), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  startCharging() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.chargingStartForm.getRawValue();
    const payload = {
      auth: {
        idTag: raw.idTag ?? '',
        authorizationId: raw.authorizationId ?? ''
      },
      meterStartWh: this.toNumber(raw.meterStartWh, 1200500),
      targetPowerW: this.toNumber(raw.targetPowerW, 11000),
      chargingProfile: { enabled: false }
    };
    this.runAction(
      'Charging started',
      this.api.startCharging(chargerId, Number(raw.connectorId), payload),
      () => {
        this.refreshSelectedCharger(chargerId);
      }
    );
  }

  stopCharging() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.chargingStopForm.getRawValue();
    const payload = {
      transactionId: raw.transactionId ?? '',
      reason: raw.reason ?? '',
      meterStopWh: this.toNumber(raw.meterStopWh, 1209500)
    };
    this.runAction('Charging stopped', this.api.stopCharging(chargerId, Number(raw.connectorId), payload), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  sendMeterValues() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.meterForm.getRawValue();
    const payload = {
      transactionId: raw.transactionId ?? '',
      samples: [
        { measurand: 'Energy.Active.Import.Register', value: this.toNumber(raw.energyWh, 0), unit: 'Wh' },
        { measurand: 'Power.Active.Import', value: this.toNumber(raw.powerW, 0), unit: 'W' },
        { measurand: 'SoC', value: this.toNumber(raw.socPercent, 0), unit: 'Percent' }
      ],
      timestamp: raw.timestamp ?? new Date().toISOString()
    };
    this.runAction(
      'Meter values queued',
      this.api.sendMeterValues(chargerId, Number(raw.connectorId), payload),
      () => {
        this.refreshSelectedCharger(chargerId);
      }
    );
  }

  setStatus() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.statusForm.getRawValue();
    const payload = {
      status: this.asConnectorStatus(raw.status),
      errorCode: raw.errorCode ?? 'NoError'
    };
    this.runAction('Status updated', this.api.setStatus(chargerId, Number(raw.connectorId), payload), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  injectFault() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.faultForm.getRawValue();
    const payload = {
      type: raw.type ?? '',
      errorCode: raw.errorCode ?? '',
      durationSec: this.toNumber(raw.durationSec, 120),
      disconnect: raw.disconnect ?? false
    };
    this.runAction(
      'Fault injected',
      this.api.injectFault(chargerId, Number(raw.connectorId), payload),
      () => {
        this.refreshSelectedCharger(chargerId);
      }
    );
  }

  clearFault() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.faultForm.getRawValue();
    const payload = {
      type: raw.type ?? ''
    };
    this.runAction('Fault cleared', this.api.clearFault(chargerId, Number(raw.connectorId), payload), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  sendHeartbeat() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    this.runAction('Heartbeat queued', this.api.sendHeartbeat(chargerId), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  updateHeartbeatInterval() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.heartbeatForm.getRawValue();
    const payload = {
      heartbeatIntervalSec: this.toNumber(raw.heartbeatIntervalSec, 20)
    };
    this.runAction('Heartbeat interval updated', this.api.setHeartbeatInterval(chargerId, payload), () => {
      this.refreshSelectedCharger(chargerId);
    });
  }

  sendOcpp() {
    const chargerId = this.selectedChargerId();
    if (!chargerId) {
      return;
    }
    const raw = this.ocppSendForm.getRawValue();
    const payload = this.safeJsonParse(raw.payload ?? '{}');
    this.runAction(
      'OCPP message sent',
      this.api.ocppSend(chargerId, {
        action: raw.action ?? '',
        payload,
        awaitResponse: raw.awaitResponse ?? true,
        timeoutMs: this.toNumber(raw.timeoutMs, 5000)
      }),
      () => {
        this.refreshSelectedCharger(chargerId);
      }
    );
  }

  openEventStream() {
    this.closeEventStream();
    const chargerId = this.selectedChargerId();
    this.eventSource = this.api.openEventsStream(chargerId || undefined);
    this.eventSource.onopen = () => {
      this.eventStreamConnected.set(true);
      this.logAction('Event stream connected.');
    };
    this.eventSource.onmessage = (event) => {
      const payload = this.safePretty(event.data);
      this.events.update((items) => [
        { at: new Date().toISOString(), type: event.type || 'EVENT', payload },
        ...items
      ].slice(0, 200));
    };
    this.eventSource.onerror = () => {
      this.eventStreamConnected.set(false);
      this.logAction('Event stream disconnected.');
    };
  }

  closeEventStream() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
      this.eventStreamConnected.set(false);
    }
  }

  addConnector() {
    this.connectors.push(this.buildConnector());
  }

  removeConnector(index: number) {
    if (this.connectors.length > 1) {
      this.connectors.removeAt(index);
    }
  }

  requestStartCharging() {
    if (!this.canStartCharging()) {
      return;
    }
    this.pendingConfirmation.set({
      action: 'startCharging',
      title: 'Start charging session',
      message: `Start charging on connector #${this.focusedConnectorId()}?`
    });
  }

  requestStopCharging() {
    if (!this.canStopCharging()) {
      return;
    }
    this.pendingConfirmation.set({
      action: 'stopCharging',
      title: 'Stop charging session',
      message: `Stop charging on connector #${this.focusedConnectorId()}?`
    });
  }

  closeConfirmation() {
    this.pendingConfirmation.set(null);
  }

  confirmPendingAction() {
    const pending = this.pendingConfirmation();
    if (!pending) {
      return;
    }
    this.pendingConfirmation.set(null);
    if (pending.action === 'startCharging') {
      this.startCharging();
      return;
    }
    this.stopCharging();
  }

  refreshSelected() {
    this.refreshSelectedCharger();
    this.fetchConnection();
  }

  getActiveTransactionCount(charger: ChargerDetails | null | undefined) {
    return charger?.runtime?.activeTransactions?.length ?? 0;
  }

  getConnectorTransaction(connectorId: number): ActiveTransaction | null {
    return this.selectedCharger()?.runtime?.activeTransactions?.find((tx) => tx.connectorId === connectorId) ?? null;
  }

  focusedConnectorId() {
    return this.toNumber(this.tapForm.get('connectorId')?.value, 1);
  }

  focusedConnector() {
    return this.selectedCharger()?.connectors?.find((connector) => connector.connectorId === this.focusedConnectorId()) ?? null;
  }

  focusedConnectorStatus() {
    return this.focusedConnector()?.status;
  }

  isChargerConnected() {
    const state = this.selectedConnection()?.connectionState || this.selectedCharger()?.connectionState;
    return state === 'CONNECTED';
  }

  isChargerConnecting() {
    const state = this.selectedConnection()?.connectionState || this.selectedCharger()?.connectionState;
    return state === 'CONNECTING';
  }

  hasFocusedTransaction() {
    return Boolean(this.getConnectorTransaction(this.focusedConnectorId()));
  }

  canConnect() {
    return Boolean(this.selectedCharger()) && !this.isChargerConnected() && !this.isChargerConnecting() && !this.isLoading();
  }

  shouldHighlightDisconnect() {
    return this.isChargerConnected() || this.isChargerConnecting();
  }

  canDisconnect() {
    return Boolean(this.selectedCharger()) && !this.isLoading() && (this.isChargerConnected() || this.isChargerConnecting());
  }

  canTap() {
    return this.isChargerConnected() && !this.hasFocusedTransaction() && !this.isLoading();
  }

  canStartPnc() {
    return this.isChargerConnected() && !this.hasFocusedTransaction() && !this.isLoading();
  }

  canStopPnc() {
    return this.isChargerConnected() && this.hasFocusedTransaction() && !this.isLoading();
  }

  canStartCharging() {
    const status = this.focusedConnectorStatus();
    return (
      this.isChargerConnected() &&
      !this.hasFocusedTransaction() &&
      !this.isLoading() &&
      (status === 'Available' || status === 'Preparing' || status === 'Finishing' || !status)
    );
  }

  shouldHighlightStartCharging() {
    return this.canStartCharging();
  }

  canStopCharging() {
    return this.isChargerConnected() && this.hasFocusedTransaction() && !this.isLoading();
  }

  shouldHighlightStopCharging() {
    return this.canStopCharging();
  }

  canSendMeterValues() {
    return this.isChargerConnected() && this.hasFocusedTransaction() && !this.isLoading();
  }

  canSetStatus() {
    return this.isChargerConnected() && !this.isLoading();
  }

  canInjectFault() {
    return this.isChargerConnected() && !this.isLoading();
  }

  canClearFault() {
    return this.isChargerConnected() && !this.isLoading() && Boolean(this.focusedConnector()?.errorCode && this.focusedConnector()?.errorCode !== 'NoError');
  }

  canSendHeartbeat() {
    return this.isChargerConnected() && !this.isLoading();
  }

  canSendOcpp() {
    return this.isChargerConnected() && !this.isLoading();
  }

  setConnectorContext(connectorId: number) {
    const transaction = this.getConnectorTransaction(connectorId);
    this.tapForm.patchValue({ connectorId });
    this.pncStartForm.patchValue({ connectorId });
    this.pncStopForm.patchValue({ connectorId });
    this.chargingStartForm.patchValue({ connectorId });
    this.chargingStopForm.patchValue({ connectorId, transactionId: transaction?.transactionId ?? this.chargingStopForm.get('transactionId')?.value });
    this.meterForm.patchValue({ connectorId, transactionId: transaction?.transactionId ?? this.meterForm.get('transactionId')?.value });
    this.statusForm.patchValue({ connectorId });
    this.faultForm.patchValue({ connectorId });
  }

  isConnectorHealthy(status?: ConnectorStatus) {
    return status === 'Available' || status === 'Charging' || status === 'Preparing' || status === 'Finishing';
  }

  isConnectorWarning(status?: ConnectorStatus) {
    return status === 'Unavailable' || status === 'SuspendedEV' || status === 'SuspendedEVSE';
  }

  private buildConnector() {
    return this.fb.group({
      connectorId: [1, Validators.required],
      type: ['CCS', Validators.required],
      maxKw: [150, Validators.required]
    });
  }

  private seedConfigForm(charger: ChargerDetails) {
    const config = charger.config;
    if (!config) {
      return;
    }
    this.configForm.patchValue({
      heartbeatIntervalSec: config.heartbeatIntervalSec ?? 60,
      meterValuesIntervalSec: config.meterValuesIntervalSec ?? 15,
      socStartPercent: config.soc?.startPercent ?? 35,
      socEndPercent: config.soc?.endPercent ?? 80
    });
  }

  private seedConnectionForm(charger: ChargerDetails) {
    const csmsUrl =
      charger.transport?.csmsUrl ||
      buildDefaultCsmsUrl(charger.ocppVersion, charger.ocppIdentity ?? '', charger.chargerId);
    this.connectionForm.patchValue({
      csmsUrl,
      reconnectEnabled: true,
      minBackoffMs: 200,
      maxBackoffMs: 5000
    });
  }

  private refreshSelectedCharger(chargerId = this.selectedChargerId()) {
    if (!chargerId) {
      return;
    }
    this.api
      .getCharger(chargerId)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (charger) => {
          this.selectedCharger.set(charger);
          this.seedConfigForm(charger);
          this.seedConnectionForm(charger);
          this.setConnectorContext(this.focusedConnectorId());
        }
      });
  }

  private buildCreatePayload(): ChargerCreateRequest {
    const raw = this.createForm.getRawValue();
    const connectors = (raw.connectors ?? []).map((connector) => ({
      connectorId: this.toNumber(connector.connectorId, 1),
      type: connector.type ?? '',
      maxKw: this.toNumber(connector.maxKw, 0)
    })) as ChargerConnector[];
    return {
      chargerId: raw.chargerId ?? '',
      ocppIdentity: raw.ocppIdentity ?? '',
      ocppVersion: this.asOcppVersion(raw.ocppVersion),
      transport: {
        role: this.asTransportRole(raw.transport?.role),
        csmsUrl: raw.transport?.csmsUrl ?? '',
        tls: {
          enabled: Boolean(raw.transport?.tls?.enabled),
          skipVerify: Boolean(raw.transport?.tls?.skipVerify)
        }
      },
      connectors,
      config: {
        heartbeatIntervalSec: this.toNumber(raw.config?.heartbeatIntervalSec, 60),
        meterValuesIntervalSec: this.toNumber(raw.config?.meterValuesIntervalSec, 15),
        soc: {
          enabled: Boolean(raw.config?.soc?.enabled),
          startPercent: this.toNumber(raw.config?.soc?.startPercent, 35),
          endPercent: this.toNumber(raw.config?.soc?.endPercent, 80),
          ratePercentPerMin: this.toNumber(raw.config?.soc?.ratePercentPerMin, 1)
        },
        metering: {
          energyWhStart: this.toNumber(raw.config?.metering?.energyWhStart, 1200000),
          powerW: this.toNumber(raw.config?.metering?.powerW, 11000),
          voltageV: this.toNumber(raw.config?.metering?.voltageV, 400),
          currentA: this.toNumber(raw.config?.metering?.currentA, 16)
        },
        clock: {
          timeZone: raw.config?.clock?.timeZone ?? 'UTC',
          driftMsPerMin: this.toNumber(raw.config?.clock?.driftMsPerMin, 0)
        },
        boot: {
          vendor: raw.config?.boot?.vendor ?? 'SimVendor',
          model: raw.config?.boot?.model ?? 'SimModel-1',
          firmwareVersion: raw.config?.boot?.firmwareVersion ?? '1.0.0'
        }
      },
      tags: {
        site: raw.tags?.site ?? '',
        shard: raw.tags?.shard ?? ''
      }
    };
  }

  private runAction<T>(label: string, request: Observable<T>, onSuccess?: (value: T) => void) {
    this.isLoading.set(true);
    this.error.set(null);
    request.pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (value: T) => {
        this.isLoading.set(false);
        this.logAction(label);
        onSuccess?.(value);
      },
      error: (err: unknown) => {
        this.isLoading.set(false);
        const message = this.describeError(err);
        this.error.set(message);
        this.logAction(`${label} failed: ${message}`);
      }
    });
  }

  private logAction(message: string) {
    const stamped = `${new Date().toLocaleTimeString()} · ${message}`;
    this.actionLog.update((items) => [stamped, ...items].slice(0, 200));
  }

  private describeError(err: unknown) {
    if (err instanceof HttpErrorResponse) {
      const backendMessage = this.extractBackendError(err.error);
      if (backendMessage) {
        return backendMessage;
      }
      if (err.status === 0) {
        return 'Network error: cannot reach API endpoint';
      }
      return `Request failed (${err.status})`;
    }
    if (err && typeof err === 'object' && 'message' in err) {
      return String((err as { message?: string }).message);
    }
    return 'Request failed';
  }

  private extractBackendError(errorBody: unknown): string | null {
    if (typeof errorBody === 'string' && errorBody.trim().length > 0) {
      return errorBody.trim();
    }
    if (errorBody && typeof errorBody === 'object') {
      const body = errorBody as Record<string, unknown>;
      const message = body['message'] ?? body['error'] ?? body['detail'];
      if (typeof message === 'string' && message.trim().length > 0) {
        return message.trim();
      }
    }
    return null;
  }

  private bumpDefaultChargerId() {
    const currentId = String(this.createForm.getRawValue().chargerId ?? '');
    const match = currentId.match(/^(.*?)(\d+)$/);
    if (!match) {
      return;
    }
    const prefix = match[1];
    const digits = match[2];
    const next = (Number(digits) + 1).toString().padStart(digits.length, '0');
    this.createForm.patchValue({ chargerId: `${prefix}${next}` });
  }

  private safeJsonParse(value: string) {
    try {
      const parsed = JSON.parse(value);
      if (parsed && typeof parsed === 'object') {
        return parsed as Record<string, unknown>;
      }
      return { value: parsed };
    } catch {
      return { raw: value };
    }
  }

  private safePretty(value: string) {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }

  private toNumber(value: unknown, fallback: number) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
  }

  private syncCreateCsmsUrl() {
    const control = this.createForm.controls.transport.controls.csmsUrl;
    const raw = this.createForm.getRawValue();
    const nextSuggested = buildDefaultCsmsUrl(raw.ocppVersion, raw.ocppIdentity ?? '', raw.chargerId ?? '');
    const current = control.value?.trim() ?? '';
    if (!current || current === this.lastSuggestedCreateCsmsUrl) {
      control.patchValue(nextSuggested, { emitEvent: false });
    }
    this.lastSuggestedCreateCsmsUrl = nextSuggested;
  }

  private asOcppVersion(value: unknown): OcppVersion {
    return value === 'OCPP201' ? 'OCPP201' : 'OCPP16J';
  }

  private asTransportRole(value: unknown): 'CP' | 'CSMS' {
    return value === 'CSMS' ? 'CSMS' : 'CP';
  }

  private asConnectorStatus(value: unknown): ConnectorStatus {
    const allowed: ConnectorStatus[] = [
      'Available',
      'Preparing',
      'Charging',
      'SuspendedEV',
      'SuspendedEVSE',
      'Finishing',
      'Unavailable',
      'Faulted'
    ];
    return allowed.includes(value as ConnectorStatus) ? (value as ConnectorStatus) : 'Available';
  }
}
