package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"api/mailer"
)

type publicServiceStatus struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Status    string `json:"status"` // "operational" or "outage"
	LatencyMS int64  `json:"latency_ms"`
}

type publicStatusResponse struct {
	Status     string                `json:"status"` // "operational", "degraded" or "outage"
	CheckedAt  string                `json:"checked_at"`
	Services   []publicServiceStatus `json:"services"`
}

// Overridable seams for tests.
var publicTCPProbe = func(ctx context.Context, host string, port int) (bool, int64) {
	started := time.Now()
	connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return false, latency
	}
	_ = connection.Close()
	return true, latency
}

var publicDBPing = func(ctx context.Context, db *sql.DB) bool {
	if db == nil {
		return false
	}
	return db.PingContext(ctx) == nil
}

var publicStatusCacheTTL = 30 * time.Second

var publicStatusCache struct {
	sync.Mutex
	at   time.Time
	body []byte
}

// PublicStatusHandler exposes a sanitized, cached subset of the supervision
// probes: service names and latencies only, no hosts, ports or versions.
func PublicStatusHandler(db *sql.DB, settings ServerSettings, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicStatusCache.Lock()
		if time.Since(publicStatusCache.at) < publicStatusCacheTTL && len(publicStatusCache.body) > 0 {
			body := publicStatusCache.body
			publicStatusCache.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		publicStatusCache.Unlock()

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		type probeResult struct {
			ok      bool
			latency int64
		}
		probe := func(host string, port int) probeResult {
			ok, latency := publicTCPProbe(ctx, host, port)
			return probeResult{ok, latency}
		}
		dbOK := publicDBPing(ctx, db)
		hbbs := probe(settings.ServerIP, settings.RendezvousPort)
		hbbr := probe(settings.ServerIP, settings.RelayPort)
		smtpOK := false
		var smtpLatency int64
		if mail != nil && mail.IsConfigured() {
			smtpOK, smtpLatency = publicTCPProbe(ctx, mail.Host, mail.Port)
		}

		apiStatus, relayStatus, smtpStatus := "operational", "operational", "operational"
		if !dbOK {
			apiStatus = "outage"
		}
		relayLatency := hbbs.latency
		if hbbr.latency > relayLatency {
			relayLatency = hbbr.latency
		}
		if !hbbs.ok || !hbbr.ok {
			relayStatus = "outage"
		}
		if !smtpOK {
			smtpStatus = "outage"
		}
		overall := "operational"
		if apiStatus != "operational" {
			overall = "outage"
		} else if relayStatus != "operational" || smtpStatus != "operational" {
			overall = "degraded"
		}

		body, err := json.Marshal(publicStatusResponse{
			Status:    overall,
			CheckedAt: time.Now().UTC().Format(time.RFC3339),
			Services: []publicServiceStatus{
				{Name: "api", Label: "API RelaisDesk", Status: apiStatus},
				{Name: "remote", Label: "Connexion à distance", Status: relayStatus, LatencyMS: relayLatency},
				{Name: "notifications", Label: "Notifications e-mail", Status: smtpStatus, LatencyMS: smtpLatency},
			},
		})
		if err != nil {
			writeJSONError(w, "Statut indisponible", http.StatusInternalServerError)
			return
		}
		publicStatusCache.Lock()
		publicStatusCache.at = time.Now()
		publicStatusCache.body = body
		publicStatusCache.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}
