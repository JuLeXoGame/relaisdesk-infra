package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ServiceRate struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Mode  string `json:"mode"`
	Cents int64  `json:"cents"`
}
type ServiceWork struct {
	ID          string `json:"id"`
	PeerID      string `json:"peer_id"`
	TargetKind  string `json:"target_kind"`
	TargetID    string `json:"target_id"`
	Label       string `json:"label"`
	Mode        string `json:"mode"`
	State       string `json:"state"`
	Paid        bool   `json:"paid"`
	ConnectedMS int64  `json:"connected_ms"`
	AmountCents int64  `json:"amount_cents"`
	CheckoutURL string `json:"checkout_url"`
}
type ServiceCatalog struct {
	Enabled bool          `json:"enabled"`
	Rates   []ServiceRate `json:"rates"`
	Work    []ServiceWork `json:"work"`
}

func serviceCatalog(token string) (*ServiceCatalog, error) {
	var c ServiceCatalog
	e := doTechnicianReq("GET", "/api/v1/technician/service-billing", token, nil, &c)
	return &c, e
}
func serviceWorkCall(token, id, action string, body any) (*ServiceWork, error) {
	method := "GET"
	path := "/api/v1/technician/service-billing"
	if id != "" {
		path += "/" + url.PathEscape(id)
	}
	if action != "" {
		path += "/" + action
		method = "POST"
	}
	if id == "" {
		method = "POST"
	}
	var w ServiceWork
	err := doTechnicianReq(method, path, token, body, &w)
	return &w, err
}
func validServiceCheckout(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Host == "checkout.stripe.com" && u.User == nil && u.Fragment == ""
}
func checkServiceEngine(binary string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil || !strings.Contains(string(output), "billing-meter-v1") {
		return errors.New("ce moteur ne mesure pas les connexions : installez le moteur RelaisDesk billing-meter-v1 avant une prestation")
	}
	return nil
}

type serviceBinding struct {
	Endpoint string `json:"endpoint"`
	Secret   string `json:"secret"`
	Peer     string `json:"peer"`
}
type servicePulse struct {
	Kind       string `json:"kind"`
	Connection string `json:"connection"`
	Sequence   int64  `json:"sequence"`
	Cumulative int64  `json:"cumulative_ms"`
	Closed     bool   `json:"closed"`
}
type serviceBridge struct {
	mu                                      sync.Mutex
	server                                  *http.Server
	path, secret, token, workID, connection string
	stopped                                 atomic.Bool
	lastPulse                               time.Time
}

var serviceBridges = struct {
	sync.Mutex
	peers map[string]*serviceBridge
}{peers: map[string]*serviceBridge{}}

func startServiceBridge(token, tokenFile string, w *ServiceWork) (*serviceBridge, error) {
	if w == nil || !numericFleetTarget(w.PeerID) || tokenFile == "" || w.State == "finished" || w.State == "cancelled" || (w.Mode == "prepaid" && !w.Paid) {
		return nil, errors.New("prestation non autorisée")
	}
	serviceBridges.Lock()
	defer serviceBridges.Unlock()
	if serviceBridges.peers[w.PeerID] != nil {
		return nil, errors.New("une prestation est déjà ouverte sur ce poste")
	}
	dir := filepath.Join(filepath.Dir(tokenFile), "prestations")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if info, e := os.Lstat(dir); e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("dossier de prestations invalide")
	}
	if err := protectServiceDirectory(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, w.PeerID+".json")
	// An abandoned binding has no usable listener; never replace a live one.
	if raw, e := os.ReadFile(path); e == nil {
		var old serviceBinding
		if len(raw) > 4096 || json.Unmarshal(raw, &old) != nil {
			return nil, errors.New("configuration de prestation existante invalide")
		}
		if serviceBindingAlive(old) {
			return nil, errors.New("une autre application gère déjà ce poste")
		}
		if e = os.Remove(path); e != nil {
			return nil, e
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		listener.Close()
		return nil, err
	}
	b := &serviceBridge{path: path, secret: hex.EncodeToString(random[:]), token: token, workID: w.ID}
	endpoint := "http://" + listener.Addr().String() + "/meter"
	b.server = &http.Server{Handler: http.HandlerFunc(b.handle), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 12 * time.Second, MaxHeaderBytes: 4096}
	raw, _ := json.Marshal(serviceBinding{Endpoint: endpoint, Secret: b.secret, Peer: w.PeerID})
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, err = f.Write(raw)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		listener.Close()
		return nil, err
	}
	serviceBridges.peers[w.PeerID] = b
	go func() { _ = b.server.Serve(listener) }()
	return b, nil
}
func serviceBindingAlive(v serviceBinding) bool {
	u, e := url.Parse(v.Endpoint)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "/meter" || u.User != nil {
		return false
	}
	req, e := http.NewRequest("POST", v.Endpoint, strings.NewReader(`{"kind":"ready"}`))
	if e != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+v.Secret)
	c := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := c.Do(req)
	if e != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == 200
}
func (b *serviceBridge) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.URL.Path != "/meter" || r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.secret)) != 1 {
		http.Error(w, "denied", 403)
		return
	}
	var p servicePulse
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "invalid", 400)
		return
	}
	if b.stopped.Load() {
		http.Error(w, "closed", 403)
		return
	}
	if p.Kind == "ready" {
		w.WriteHeader(200)
		return
	}
	if p.Kind != "pulse" {
		http.Error(w, "invalid", 400)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped.Load() {
		http.Error(w, "closed", 403)
		return
	}
	if b.connection != "" && b.connection != p.Connection && time.Since(b.lastPulse) < 10*time.Second {
		http.Error(w, "one connection at a time", 409)
		return
	}
	_, err := serviceWorkCall(b.token, b.workID, "pulse", map[string]any{"connection": p.Connection, "sequence": p.Sequence, "cumulative_ms": p.Cumulative, "closed": p.Closed})
	if err != nil {
		http.Error(w, "meter unavailable", 409)
		return
	}
	if p.Closed {
		b.connection = ""
	} else {
		b.connection = p.Connection
	}
	b.lastPulse = time.Now()
	w.WriteHeader(200)
}
func (b *serviceBridge) stop() {
	if b == nil {
		return
	}
	b.stopped.Store(true)
	b.mu.Lock()
	b.mu.Unlock()
	_ = b.server.Close()
	serviceBridges.Lock()
	defer serviceBridges.Unlock()
	for peer, active := range serviceBridges.peers {
		if active == b {
			delete(serviceBridges.peers, peer)
			_ = os.Remove(b.path)
		}
	}
}
func serviceDirectAllowed(peer string) error {
	serviceBridges.Lock()
	defer serviceBridges.Unlock()
	if serviceBridges.peers[peer] != nil {
		return fmt.Errorf("terminez la prestation de ce poste avant une connexion directe")
	}
	return nil
}

func stopServiceBridges(tokenFile string) {
	dir := filepath.Join(filepath.Dir(tokenFile), "prestations")
	serviceBridges.Lock()
	var closing []*serviceBridge
	for _, b := range serviceBridges.peers {
		if filepath.Dir(b.path) == dir {
			b.stopped.Store(true)
			closing = append(closing, b)
		}
	}
	serviceBridges.Unlock()
	for _, b := range closing {
		go b.stop()
	}
}

func cleanupAbandonedServiceBindings(tokenFile string) {
	dir := filepath.Join(filepath.Dir(tokenFile), "prestations")
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		peer := strings.TrimSuffix(file.Name(), ".json")
		if !numericFleetTarget(peer) {
			continue
		}
		path := filepath.Join(dir, file.Name())
		meta, e := os.Lstat(path)
		if e != nil || !meta.Mode().IsRegular() || meta.Size() > 4096 {
			continue
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			continue
		}
		var v serviceBinding
		if json.Unmarshal(raw, &v) == nil && v.Peer == peer && !serviceBindingAlive(v) {
			_ = os.Remove(path)
		}
	}
}
