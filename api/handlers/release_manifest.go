package handlers

import (
	"log"
	"net/http"
	"strings"

	"api/config"
	"api/releasemanifest"
)

func PublicReleaseManifestHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if cfg == nil || strings.TrimSpace(cfg.ReleaseManifestPath) == "" || strings.TrimSpace(cfg.ReleasePublicKey) == "" {
			log.Printf("[ReleaseManifest] Configuration incomplète: path=%q, key=%q", cfg.ReleaseManifestPath, cfg.ReleasePublicKey)
			writeJSONError(w, "Canal de mise à jour indisponible", http.StatusServiceUnavailable)
			return
		}
		manifest, err := releasemanifest.LoadVerified(cfg.ReleaseManifestPath, cfg.ReleasePublicKey)
		if err != nil {
			log.Printf("[ReleaseManifest] Erreur vérification manifeste (path=%q): %v", cfg.ReleaseManifestPath, err)
			writeJSONError(w, "Canal de mise à jour non vérifiable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		writeJSON(w, http.StatusOK, manifest)
	}
}
