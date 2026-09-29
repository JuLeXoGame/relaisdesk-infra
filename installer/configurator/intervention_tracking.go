package main

// This bridge receives lifecycle events from the locally launched, RelaisDesk
// patched RustDesk engine.  It deliberately contains a technician session
// token, not the network-authorisation token read by RustDesk.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type interventionBinding struct {
	Endpoint string `json:"endpoint"`
	Secret   string `json:"secret"`
	Peer     string `json:"peer"`
}

type interventionEvent struct {
	Event string `json:"event"`
}

type interventionBridge struct {
	mu                        sync.Mutex
	server                    *http.Server
	path, secret, token, work string
	peer                      string
	connected, finalizing     bool
	finalized                 bool
	stopped                   bool
}

// logBridgeEvent appends one timestamped line to the per-peer bridge log kept
// next to the binding file. Logging is best effort and never fails tracking.
func (b *interventionBridge) logBridgeEvent(format string, args ...any) {
	if b == nil {
		return
	}
	dir := filepath.Dir(b.path)
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	logPath := filepath.Join(dir, b.peer+".log")
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(f, "%s peer=%s work=%s %s\n",
		time.Now().UTC().Format(time.RFC3339), b.peer, b.work, fmt.Sprintf(format, args...))
	_ = f.Close()
}

var interventionBridges = struct {
	sync.Mutex
	peers map[string]*interventionBridge
}{peers: map[string]*interventionBridge{}}

func validInterventionID(id string) bool {
	if !strings.HasPrefix(id, "INT-") || len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, c := range id[4:] {
		if !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '-' {
			return false
		}
	}
	return true
}

func startInterventionBridge(token, tokenFile, peer, interventionID string) (*interventionBridge, error) {
	if strings.TrimSpace(token) == "" || tokenFile == "" || !numericFleetTarget(peer) || !validInterventionID(interventionID) {
		return nil, errors.New("suivi d'intervention invalide")
	}
	interventionBridges.Lock()
	defer interventionBridges.Unlock()
	if existing := interventionBridges.peers[peer]; existing != nil {
		if err := existing.rebind(interventionID); err != nil {
			return nil, err
		}
		return existing, nil
	}
	dir := filepath.Join(filepath.Dir(tokenFile), "interventions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("dossier de suivi invalide")
	}
	if err := protectServiceDirectory(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, peer+".json")
	// Recovery after an abnormal application exit is safe only in the
	// protected per-user directory; never follow or remove a symbolic link.
	if old, e := os.Lstat(path); e == nil {
		if !old.Mode().IsRegular() || old.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("fichier de suivi invalide")
		}
		if e = os.Remove(path); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		_ = listener.Close()
		return nil, err
	}
	b := &interventionBridge{path: path, secret: hex.EncodeToString(random[:]), token: token, work: interventionID, peer: peer}
	b.server = &http.Server{Handler: http.HandlerFunc(b.handle), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 12 * time.Second, MaxHeaderBytes: 4096}
	raw, err := json.Marshal(interventionBinding{Endpoint: "http://" + listener.Addr().String() + "/intervention", Secret: b.secret, Peer: peer})
	if err == nil {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if e != nil {
			err = e
		} else {
			_, err = f.Write(raw)
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	interventionBridges.peers[peer] = b
	go func() { _ = b.server.Serve(listener) }()
	// Fresh log per session; the previous run stays readable until now.
	_ = os.Remove(filepath.Join(dir, peer+".log"))
	b.logBridgeEvent("bridge started")
	return b, nil
}

func (b *interventionBridge) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/intervention" || r.Header.Get("Origin") != "" ||
		subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.secret)) != 1 {
		http.Error(w, "denied", http.StatusForbidden)
		return
	}
	var event interventionEvent
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
	d.DisallowUnknownFields()
	if d.Decode(&event) != nil || d.Decode(&struct{}{}) != io.EOF || (event.Event != "connected" && event.Event != "closed") {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		http.Error(w, "closed", http.StatusGone)
		return
	}
	if event.Event == "connected" {
		b.connected = true
		b.mu.Unlock()
		b.logBridgeEvent("event connected")
		w.WriteHeader(http.StatusOK)
		return
	}
	if b.finalizing {
		b.mu.Unlock()
		// Ask the engine to retry rather than acknowledging a second close
		// event before the first API completion has definitely succeeded.
		http.Error(w, "completion pending", http.StatusServiceUnavailable)
		return
	}
	b.finalizing = true
	connected := b.connected
	token, interventionID := b.token, b.work
	b.mu.Unlock()

	var err error
	if connected {
		err = technicianCompleteIntervention(token, interventionID, "Session d'accès à distance terminée automatiquement")
	} else {
		err = technicianCancelIntervention(token, interventionID)
	}
	if err != nil {
		b.mu.Lock()
		b.finalizing = false
		b.mu.Unlock()
		b.logBridgeEvent("event closed: api error: %v", err)
		http.Error(w, "completion unavailable", http.StatusServiceUnavailable)
		return
	}
	b.mu.Lock()
	b.finalized = true
	b.finalizing = false
	b.mu.Unlock()
	b.logBridgeEvent("event closed: finalized connected=%v", connected)
	w.WriteHeader(http.StatusOK)
	// The bridge persists after finalizing so a later session to the same
	// peer (retry or reconnect) stays tracked; stop() settles and cleans up.
}

// rebind moves a settled bridge to a new intervention so a later connection
// to the same peer stays tracked. It refuses while a session is active or a
// completion is in flight.
func (b *interventionBridge) rebind(interventionID string) error {
	if b == nil || !validInterventionID(interventionID) {
		return errors.New("suivi d'intervention invalide")
	}
	b.mu.Lock()
	stopped, finalizing, finalized := b.stopped, b.finalizing, b.finalized
	if !stopped && !finalizing && finalized {
		b.work = interventionID
		b.connected = false
		b.finalized = false
	}
	b.mu.Unlock()
	switch {
	case stopped:
		return errors.New("suivi d'intervention invalide")
	case finalizing:
		return errors.New("clôture en cours, réessayez")
	case !finalized:
		return errors.New("une intervention est déjà suivie sur ce poste")
	}
	b.logBridgeEvent("bridge rebound to work=%s", interventionID)
	return nil
}

func (b *interventionBridge) cancelBeforeLaunch() {
	if b == nil {
		return
	}
	_ = technicianCancelIntervention(b.token, b.work)
	b.stop()
}

func (b *interventionBridge) stop() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	b.stopped = true
	connected, finalized := b.connected, b.finalized
	token, interventionID := b.token, b.work
	b.mu.Unlock()
	if !finalized {
		// The engine never delivered its final event (it may have exited
		// before flushing, or the application is quitting): settle the
		// fiche instead of leaving it open forever.
		var err error
		if connected {
			err = technicianCompleteIntervention(token, interventionID, "Session d'accès à distance terminée automatiquement")
		} else {
			err = technicianCancelIntervention(token, interventionID)
		}
		if err != nil {
			b.logBridgeEvent("stop: settle failed connected=%v: %v", connected, err)
		} else {
			b.logBridgeEvent("stop: settled connected=%v", connected)
		}
	} else {
		b.logBridgeEvent("stop: already finalized")
	}
	_ = b.server.Close()
	interventionBridges.Lock()
	if interventionBridges.peers[b.peer] == b {
		delete(interventionBridges.peers, b.peer)
		_ = os.Remove(b.path)
	}
	interventionBridges.Unlock()
}

func stopInterventionBridges(tokenFile string) {
	dir := filepath.Join(filepath.Dir(tokenFile), "interventions")
	interventionBridges.Lock()
	closing := make([]*interventionBridge, 0)
	for _, b := range interventionBridges.peers {
		if filepath.Dir(b.path) == dir {
			closing = append(closing, b)
		}
	}
	interventionBridges.Unlock()
	for _, b := range closing {
		b.stop()
	}
}

func cleanupAbandonedInterventionBindings(tokenFile string) {
	dir := filepath.Join(filepath.Dir(tokenFile), "interventions")
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

// launchTrackedIntervention keeps the connection lifecycle inside the patched
// RustDesk engine.  Waiting on the launcher process is unsafe because RustDesk
// may hand a request to an existing instance and exit immediately.
func launchTrackedIntervention(token, tokenFile, binary, peer, interventionID string, password ...string) error {
	b, err := startInterventionBridge(token, tokenFile, peer, interventionID)
	if err != nil {
		return err
	}
	if _, err = launchRustDeskSessionCmd(binary, peer, password...); err != nil {
		b.cancelBeforeLaunch()
		return err
	}
	return nil
}
