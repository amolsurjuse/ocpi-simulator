package fleet

import (
	"sync"
	"time"
)

type EventHub struct {
	mu        sync.RWMutex
	clients   map[chan Event]struct{}
	broadcast chan Event
	metrics   *Metrics
}

func NewEventHub() *EventHub {
	return &EventHub{
		clients:   make(map[chan Event]struct{}),
		broadcast: make(chan Event, 512),
		metrics:   NewMetrics(),
	}
}

func (h *EventHub) Run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case event := <-h.broadcast:
			h.metrics.RecordOut()
			h.mu.RLock()
			for ch := range h.clients {
				select {
				case ch <- event:
				default:
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *EventHub) Publish(event Event) {
	select {
	case h.broadcast <- event:
	default:
	}
}

func (h *EventHub) Subscribe() chan Event {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *EventHub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *EventHub) Metrics() *Metrics {
	return h.metrics
}

type Metrics struct {
	mu        sync.Mutex
	seconds   [60]int
	lastSec   int64
	lastClean time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{lastSec: time.Now().Unix()}
}

func (m *Metrics) RecordOut() {
	m.mu.Lock()
	defer m.mu.Unlock()

	sec := time.Now().Unix()
	if sec != m.lastSec {
		m.advance(sec)
	}
	idx := int(sec % int64(len(m.seconds)))
	m.seconds[idx]++
}

func (m *Metrics) advance(sec int64) {
	if sec-m.lastSec >= int64(len(m.seconds)) {
		for i := range m.seconds {
			m.seconds[i] = 0
		}
	} else {
		for s := m.lastSec + 1; s <= sec; s++ {
			idx := int(s % int64(len(m.seconds)))
			m.seconds[idx] = 0
		}
	}
	m.lastSec = sec
}

func (m *Metrics) RatePerSec() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	sec := time.Now().Unix()
	if sec != m.lastSec {
		m.advance(sec)
	}

	sum := 0
	for _, v := range m.seconds {
		sum += v
	}
	return sum / len(m.seconds)
}
