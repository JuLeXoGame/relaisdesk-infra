package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"sort"
	"time"

	"api/mailer"
)

type SystemComponentStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
}

type SystemStatusResponse struct {
	Status     string                  `json:"status"`
	CheckedAt  string                  `json:"checked_at"`
	Components []SystemComponentStatus `json:"components"`
}

// AdminSystemStatusHandler checks the dependencies required to sell and use
// RelaisDesk. It deliberately returns no credentials or internal file paths.
func AdminSystemStatusHandler(db *sql.DB, settings ServerSettings, mail *mailer.Mailer, signerReady bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		components := probeSystemComponents(ctx, db, settings, mail, signerReady)
		overall := "healthy"
		for _, component := range components {
			if component.Status != "healthy" {
				overall = "degraded"
				break
			}
		}
		writeJSON(w, http.StatusOK, SystemStatusResponse{
			Status: overall, CheckedAt: time.Now().UTC().Format(time.RFC3339), Components: components,
		})
	}
}

func probeSystemComponents(ctx context.Context, db *sql.DB, settings ServerSettings, mail *mailer.Mailer, signerReady bool) []SystemComponentStatus {
	type result struct {
		component SystemComponentStatus
	}
	results := make(chan result, 5)

	go func() {
		started := time.Now()
		status := SystemComponentStatus{Name: "database", Status: "healthy", Message: "base de données disponible"}
		if db == nil || db.PingContext(ctx) != nil {
			status.Status = "unhealthy"
			status.Message = "base de données indisponible"
		}
		status.LatencyMS = time.Since(started).Milliseconds()
		results <- result{status}
	}()

	go func() {
		results <- result{probeTCPComponent(ctx, "hbbs", settings.ServerIP, settings.RendezvousPort)}
	}()
	go func() {
		results <- result{probeTCPComponent(ctx, "hbbr", settings.ServerIP, settings.RelayPort)}
	}()
	go func() {
		status := SystemComponentStatus{Name: "network_signer", Status: "healthy", Message: "signature des autorisations disponible"}
		if !signerReady {
			status.Status = "unhealthy"
			status.Message = "signature des autorisations indisponible"
		}
		results <- result{status}
	}()
	go func() {
		if mail == nil || !mail.IsConfigured() {
			results <- result{SystemComponentStatus{Name: "smtp", Status: "unhealthy", Message: "service email non configuré"}}
			return
		}
		results <- result{probeTCPComponent(ctx, "smtp", mail.Host, mail.Port)}
	}()

	components := make([]SystemComponentStatus, 0, 5)
	for len(components) < 5 {
		select {
		case item := <-results:
			components = append(components, item.component)
		case <-ctx.Done():
			components = append(components, SystemComponentStatus{Name: "probe_timeout", Status: "unhealthy", Message: "délai de supervision dépassé"})
			for len(components) < 5 {
				components = append(components, SystemComponentStatus{Name: fmt.Sprintf("unknown_%d", len(components)), Status: "unhealthy", Message: "contrôle interrompu"})
			}
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Name < components[j].Name })
	return components
}

func probeTCPComponent(ctx context.Context, name, host string, port int) SystemComponentStatus {
	status := SystemComponentStatus{Name: name, Status: "healthy", Message: "service accessible"}
	started := time.Now()
	connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	status.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		status.Status = "unhealthy"
		status.Message = "service inaccessible"
		return status
	}
	_ = connection.Close()
	return status
}
