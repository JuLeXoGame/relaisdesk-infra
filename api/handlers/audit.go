package handlers

import (
	"log"
	"net/http"
	"time"

	"api/middleware"
)

// auditLog records a sensitive mutation (admin sessions, licence/order/team
// actions, bulk key exports) with the caller IP. Invoice handlers already
// log this way; this helper extends the same trail to the other privileged
// endpoints for incident detection and forensics.
func auditLog(r *http.Request, action, detail string) {
	log.Printf("[AUDIT %s] %s (IP %s, %s)", action, detail,
		middleware.GetClientIP(r), time.Now().UTC().Format(time.RFC3339))
}
