package cloud

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

func (p *Pool) Metrics(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics" || r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	p.mu.Lock()
	list := make([]*Tenant, 0, len(p.tenants))
	for _, t := range p.tenants {
		list = append(list, t)
	}
	p.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	running, pending, unhealthy, failed := 0, 0, 0, 0
	lag, age := float64(0), float64(0)
	for _, t := range list {
		if t.failed.Load() {
			failed++
		}
		m, err := t.App.Store.Monitor(ctx, time.Now())
		if err != nil {
			http.Error(w, "metrics collection unavailable", 503)
			return
		}
		running += m.Running
		pending += m.Pending
		unhealthy += m.Unhealthy
		lag = max(lag, m.LagSeconds)
		age = max(age, m.DeliveryAgeSeconds)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, "ding_cloud_workspaces %d\nding_cloud_failed_workers %d\nding_cloud_running_watches %d\nding_cloud_pending_deliveries %d\nding_cloud_unhealthy_entities %d\nding_cloud_scheduler_lag_seconds_max %g\nding_cloud_pending_delivery_age_seconds_max %g\n", len(list), failed, running, pending, unhealthy, lag, age)
}
func operatorListener(address string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("operator metrics must bind an explicit loopback IP")
	}
	return net.Listen("tcp", address)
}
