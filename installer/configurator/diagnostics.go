package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type DiagnosticCheck struct {
	Name    string
	OK      bool
	Message string
	Latency time.Duration
}

type DiagnosticReport struct {
	CheckedAt time.Time
	Checks    []DiagnosticCheck
}

func (r DiagnosticReport) OK() bool {
	if len(r.Checks) == 0 {
		return false
	}
	for _, check := range r.Checks {
		if !check.OK {
			return false
		}
	}
	return true
}

func (r DiagnosticReport) String() string {
	var output strings.Builder
	status := "ANOMALIE DÉTECTÉE"
	if r.OK() {
		status = "TOUS LES CONTRÔLES SONT VALIDÉS"
	}
	fmt.Fprintf(&output, "Diagnostic de connexion RelaisDesk — %s\n%s\n\n", r.CheckedAt.Local().Format("02/01/2006 15:04:05"), status)
	for _, check := range r.Checks {
		icon := "❌"
		if check.OK {
			icon = "✅"
		}
		latency := ""
		if check.Latency > 0 {
			latency = fmt.Sprintf(" (%d ms)", check.Latency.Milliseconds())
		}
		fmt.Fprintf(&output, "%s %s%s — %s\n", icon, check.Name, latency, check.Message)
	}
	output.WriteString("\nCe rapport ne contient ni clé de licence, ni jeton, ni clé privée.")
	return output.String()
}

func runConnectivityDiagnostics(ctx context.Context, apiBase string, activation *ActivationResponse) DiagnosticReport {
	report := DiagnosticReport{CheckedAt: time.Now()}
	report.Checks = append(report.Checks, checkAPIHealth(ctx, apiBase))

	if activation == nil {
		report.Checks = append(report.Checks, DiagnosticCheck{Name: "Configuration du serveur", Message: "configuration serveur indisponible"})
		return report
	}

	host := strings.TrimSpace(activation.ServerIP)
	report.Checks = append(report.Checks, checkDNS(ctx, host))
	report.Checks = append(report.Checks, checkTCP(ctx, "Serveur de rendez-vous (hbbs)", host, activation.RendezvousPort))
	report.Checks = append(report.Checks, checkTCP(ctx, "Serveur de relais (hbbr)", host, activation.RelayPort))

	configOK := validateRendezvousHost(host) == nil &&
		validateRendezvousPort(activation.RendezvousPort) == nil &&
		validateRendezvousPort(activation.RelayPort) == nil &&
		validateRustDeskPublicKey(activation.PublicKey) == nil
	configMessage := "hôte, ports et clé publique valides"
	if !configOK {
		configMessage = "configuration reçue invalide"
	}
	report.Checks = append(report.Checks, DiagnosticCheck{Name: "Configuration du client RustDesk", OK: configOK, Message: configMessage})
	return report
}

func checkAPIHealth(ctx context.Context, apiBase string) DiagnosticCheck {
	check := DiagnosticCheck{Name: "Serveur d'API RelaisDesk"}
	base, err := url.Parse(strings.TrimSpace(apiBase))
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil {
		check.Message = "adresse API invalide"
		return check
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v1/health"
	base.RawQuery = ""
	base.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		check.Message = "requête de contrôle impossible"
		return check
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", PRODUCT_NAME+"-Diagnostic/1.0")
	started := time.Now()
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	check.Latency = time.Since(started)
	if err != nil {
		check.Message = "API inaccessible ou certificat TLS refusé"
		return check
	}
	defer resp.Body.Close()

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil || resp.StatusCode != http.StatusOK || payload.Status != "healthy" {
		check.Message = fmt.Sprintf("réponse de santé invalide (HTTP %d)", resp.StatusCode)
		return check
	}
	check.OK = true
	check.Message = "API et base de données disponibles"
	return check
}

func checkDNS(ctx context.Context, host string) DiagnosticCheck {
	check := DiagnosticCheck{Name: "Résolution DNS du serveur"}
	if validateRendezvousHost(host) != nil {
		check.Message = "nom de serveur invalide"
		return check
	}
	if net.ParseIP(host) != nil {
		check.OK = true
		check.Message = "adresse IP valide"
		return check
	}
	started := time.Now()
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	check.Latency = time.Since(started)
	if err != nil || len(addresses) == 0 {
		check.Message = "nom de serveur introuvable"
		return check
	}
	check.OK = true
	check.Message = "nom de serveur résolu"
	return check
}

func checkTCP(ctx context.Context, name, host string, port int) DiagnosticCheck {
	check := DiagnosticCheck{Name: name}
	if validateRendezvousHost(host) != nil || validateRendezvousPort(port) != nil {
		check.Message = "adresse ou port invalide"
		return check
	}
	started := time.Now()
	connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	check.Latency = time.Since(started)
	if err != nil {
		check.Message = "port TCP inaccessible"
		return check
	}
	_ = connection.Close()
	check.OK = true
	check.Message = "port TCP accessible"
	return check
}
