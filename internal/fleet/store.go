package fleet

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu       sync.RWMutex
	chargers map[string]*Charger
	jobs     map[string]*BulkJob
}

func NewStore() *Store {
	return &Store{
		chargers: make(map[string]*Charger),
		jobs:     make(map[string]*BulkJob),
	}
}

func (s *Store) AddCharger(charger Charger) (Charger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if charger.ChargerID == "" {
		return Charger{}, errors.New("chargerId is required")
	}
	if _, exists := s.chargers[charger.ChargerID]; exists {
		return Charger{}, errors.New("charger already exists")
	}

	applyDefaults(&charger)
	s.chargers[charger.ChargerID] = &charger
	return charger, nil
}

func (s *Store) UpdateCharger(charger Charger) (Charger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.chargers[charger.ChargerID]
	if !ok {
		return Charger{}, errors.New("charger not found")
	}

	charger.CreatedAt = current.CreatedAt
	charger.UpdatedAt = time.Now().UTC()
	charger.Runtime = current.Runtime
	charger.ConnectionState = current.ConnectionState
	charger.ConnectionRemote = current.ConnectionRemote
	charger.ConnectionSince = current.ConnectionSince

	applyDefaults(&charger)
	s.chargers[charger.ChargerID] = &charger
	return charger, nil
}

func (s *Store) GetCharger(id string) (Charger, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	charger, ok := s.chargers[id]
	if !ok {
		return Charger{}, false
	}
	return cloneCharger(charger), true
}

func (s *Store) RemoveCharger(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.chargers[id]; !ok {
		return false
	}
	delete(s.chargers, id)
	return true
}

func (s *Store) ListChargers() []Charger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Charger, 0, len(s.chargers))
	for _, charger := range s.chargers {
		items = append(items, cloneCharger(charger))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ChargerID < items[j].ChargerID
	})
	return items
}

func (s *Store) UpdateConnectionState(id, state, remote string, since *time.Time) (Charger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	charger, ok := s.chargers[id]
	if !ok {
		return Charger{}, errors.New("charger not found")
	}
	charger.ConnectionState = state
	charger.ConnectionRemote = remote
	charger.ConnectionSince = since
	charger.UpdatedAt = time.Now().UTC()
	return cloneCharger(charger), nil
}

func (s *Store) UpdateRuntime(id string, update func(runtime *Runtime)) (Charger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	charger, ok := s.chargers[id]
	if !ok {
		return Charger{}, errors.New("charger not found")
	}
	update(&charger.Runtime)
	charger.UpdatedAt = time.Now().UTC()
	return cloneCharger(charger), nil
}

func (s *Store) PatchConfig(id string, patch func(config *ChargerConfig)) (Charger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	charger, ok := s.chargers[id]
	if !ok {
		return Charger{}, errors.New("charger not found")
	}
	patch(&charger.Config)
	charger.UpdatedAt = time.Now().UTC()
	return cloneCharger(charger), nil
}

func (s *Store) FindByTag(key, value string) []Charger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := []Charger{}
	for _, charger := range s.chargers {
		if charger.Tags != nil && charger.Tags[key] == value {
			items = append(items, cloneCharger(charger))
		}
	}
	return items
}

func (s *Store) FindByOCPPVersion(version string) []Charger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := []Charger{}
	for _, charger := range s.chargers {
		if strings.EqualFold(charger.OCPPVersion, version) {
			items = append(items, cloneCharger(charger))
		}
	}
	return items
}

func (s *Store) FindByOCPPIdentity(identity string) (Charger, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, charger := range s.chargers {
		if charger.OCPPIdentity == identity {
			return cloneCharger(charger), true
		}
	}
	return Charger{}, false
}

func (s *Store) SetJob(job BulkJob) BulkJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobCopy := job
	s.jobs[job.JobID] = &jobCopy
	return jobCopy
}

func (s *Store) UpdateJob(id string, update func(job *BulkJob)) (BulkJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return BulkJob{}, false
	}
	update(job)
	job.UpdatedAt = time.Now().UTC()
	return *job, true
}

func (s *Store) GetJob(id string) (BulkJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[id]
	if !ok {
		return BulkJob{}, false
	}
	return *job, true
}

func applyDefaults(charger *Charger) {
	now := time.Now().UTC()
	if charger.CreatedAt.IsZero() {
		charger.CreatedAt = now
	}
	charger.UpdatedAt = now

	if charger.OCPPVersion == "" {
		charger.OCPPVersion = "OCPP16J"
	}
	if charger.OCPPIdentity == "" {
		charger.OCPPIdentity = charger.ChargerID
	}
	if charger.Transport.Role == "" {
		charger.Transport.Role = "CP"
	}
	if charger.ConnectionState == "" {
		charger.ConnectionState = "DISCONNECTED"
	}
	if charger.Config.HeartbeatIntervalSec == 0 {
		charger.Config.HeartbeatIntervalSec = 60
	}
	if charger.Config.MeterValuesIntervalSec == 0 {
		charger.Config.MeterValuesIntervalSec = 15
	}
	if charger.Config.Clock.TimeZone == "" {
		charger.Config.Clock.TimeZone = "UTC"
	}
	if charger.Config.Boot.Vendor == "" {
		charger.Config.Boot.Vendor = "SimVendor"
	}
	if charger.Config.Boot.Model == "" {
		charger.Config.Boot.Model = "SimModel"
	}
	if len(charger.Connectors) == 0 {
		charger.Connectors = []Connector{{ConnectorID: 1, Type: "CCS", MaxKw: 50, Status: "Available", ErrorCode: "NoError"}}
	}

	for i := range charger.Connectors {
		if charger.Connectors[i].Status == "" {
			charger.Connectors[i].Status = "Available"
		}
		if charger.Connectors[i].ErrorCode == "" {
			charger.Connectors[i].ErrorCode = "NoError"
		}
	}

	if charger.Tags == nil {
		charger.Tags = map[string]string{}
	}
	if charger.Runtime.ActiveTransactions == nil {
		charger.Runtime.ActiveTransactions = []Transaction{}
	}
}

func cloneCharger(charger *Charger) Charger {
	copyCharger := *charger
	if charger.Tags != nil {
		copyCharger.Tags = map[string]string{}
		for k, v := range charger.Tags {
			copyCharger.Tags[k] = v
		}
	}
	if charger.Connectors != nil {
		copyCharger.Connectors = make([]Connector, len(charger.Connectors))
		copy(copyCharger.Connectors, charger.Connectors)
	}
	if charger.Runtime.ActiveTransactions != nil {
		copyCharger.Runtime.ActiveTransactions = make([]Transaction, len(charger.Runtime.ActiveTransactions))
		copy(copyCharger.Runtime.ActiveTransactions, charger.Runtime.ActiveTransactions)
	}
	return copyCharger
}

func NewJobID(prefix string) string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return prefix + "-" + hex.EncodeToString(buf)
}
