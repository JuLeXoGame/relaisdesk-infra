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
	Status    string                `json:"status"` // "operational", "degraded" or "outage"
	CheckedAt string                `json:"checked_at"`
	Services  []publicServiceStatus `json:"services"`
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
	at     time.Time
	body   []byte
	flight *publicStatusFlight
}

type publicStatusFlight struct {
	done chan struct{}
	body []byte
	err  error
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
		flight := publicStatusCache.flight
		if flight == nil {
			flight = &publicStatusFlight{done: make(chan struct{})}
			publicStatusCache.flight = flight
			// One bounded refresh survives the cancellation of any individual caller.
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				body, err := probePublicStatus(ctx, db, settings, mail)
				publicStatusCache.Lock()
				flight.body, flight.err = body, err
				if err == nil {
					publicStatusCache.at, publicStatusCache.body = time.Now(), body
				}
				publicStatusCache.flight = nil
				close(flight.done)
				publicStatusCache.Unlock()
			}()
		}
		publicStatusCache.Unlock()
		select {
		case <-r.Context().Done():
			return
		case <-flight.done:
			if flight.err != nil {
				writeJSONError(w, "Statut indisponible", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(flight.body)
		}
	}
}

func probePublicStatus(ctx context.Context, db *sql.DB, settings ServerSettings, mail *mailer.Mailer) ([]byte, error) {
	type probeResult struct {
		index   int
		ok      bool
		latency int64
	}
	results := make(chan probeResult, 4)
	tcpProbe, dbPing := publicTCPProbe, publicDBPing
	go func() { results <- probeResult{index: 0, ok: dbPing(ctx, db)} }()
	probe := func(index int, host string, port int) {
		ok, latency := tcpProbe(ctx, host, port)
		results <- probeResult{index, ok, latency}
	}
	go probe(1, settings.ServerIP, settings.RendezvousPort)
	go probe(2, settings.ServerIP, settings.RelayPort)
	count := 3
	if mail != nil && mail.IsConfigured() {
		count++
		go probe(3, mail.Host, mail.Port)
	}
	var values [4]probeResult
collect:
	for i := 0; i < count; i++ {
		select {
		case result := <-results:
			values[result.index] = result
		case <-ctx.Done():
			break collect
		}
	}
	dbOK := values[0].ok
	hbbs, hbbr := values[1], values[2]
	smtpOK, smtpLatency := values[3].ok, values[3].latency

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

	return json.Marshal(publicStatusResponse{
		Status:    overall,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Services: []publicServiceStatus{
			{Name: "api", Label: "API RelaisDesk", Status: apiStatus},
			{Name: "remote", Label: "Connexion à distance", Status: relayStatus, LatencyMS: relayLatency},
			{Name: "notifications", Label: "Notifications e-mail", Status: smtpStatus, LatencyMS: smtpLatency},
		},
	})
}
