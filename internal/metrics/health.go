package metrics

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// HealthChecker tracks the forwarder's readiness state.
type HealthChecker struct {
	natsConnected atomic.Bool
	subscribed    atomic.Bool
}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{}
}

func (h *HealthChecker) SetNATSConnected(connected bool) {
	h.natsConnected.Store(connected)
}

func (h *HealthChecker) SetSubscribed(subscribed bool) {
	h.subscribed.Store(subscribed)
}

type healthStatus struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (h *HealthChecker) IsAlive() bool {
	return true // process is alive if handler runs
}

func (h *HealthChecker) IsReady() (bool, map[string]string) {
	checks := map[string]string{}
	natsOK := h.natsConnected.Load()
	subOK := h.subscribed.Load()
	if natsOK {
		checks["nats"] = "connected"
	} else {
		checks["nats"] = "disconnected"
	}
	if subOK {
		checks["subscriptions"] = "active"
	} else {
		checks["subscriptions"] = "inactive"
	}
	return natsOK && subOK, checks
}

func (h *HealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(healthStatus{Status: "ok"})
	}
}

func (h *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ready, checks := h.IsReady()
		if ready {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(healthStatus{Status: "ok", Checks: checks})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(healthStatus{Status: "unavailable", Checks: checks})
		}
	}
}
