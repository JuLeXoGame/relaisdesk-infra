package main

import (
	"fmt"
	"net"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	// Une autre URL peut uniquement être injectée à la compilation avec -ldflags
	// pour une version de test explicitement distincte. L'environnement local ne
	// doit jamais pouvoir rediriger un code Viewer vers un autre serveur.
	APIURL      = "https://api.relaisdesk.fr"
	APP_VERSION = "1.0.0"
)

const (
	PRODUCT_NAME             = "RelaisDesk Viewer"
	RUSTDESK_RENDEZVOUS_PORT = 21116
	RUSTDESK_RELAY_PORT      = 21117
)

func isCorruptedRustDeskConfig(content []byte) bool {
	var config struct {
		KeyPair [][]int `toml:"key_pair"`
	}
	if _, err := toml.Decode(string(content), &config); err != nil {
		return false
	}
	return len(config.KeyPair) == 2 && len(config.KeyPair[0]) == 0 && len(config.KeyPair[1]) == 0
}

func validateRendezvousHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("serveur rendez-vous manquant")
	}
	if len(host) > 253 {
		return fmt.Errorf("serveur rendez-vous trop long")
	}
	if net.ParseIP(host) != nil {
		return nil
	}

	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("serveur rendez-vous invalide")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
				(char < '0' || char > '9') && char != '-' {
				return fmt.Errorf("serveur rendez-vous invalide")
			}
		}
	}
	return nil
}

func validateRendezvousPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port rendez-vous invalide")
	}
	return nil
}

func validateRustDeskPublicKey(key string) error {
	if strings.TrimSpace(key) != key || len(key) < 32 || len(key) > 128 {
		return fmt.Errorf("clé publique RustDesk invalide")
	}
	for _, char := range key {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && !strings.ContainsRune("+/=_-", char) {
			return fmt.Errorf("clé publique RustDesk invalide")
		}
	}
	return nil
}
