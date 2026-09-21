package handlers

import (
	dbpkg "database"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"api/config"
	"api/mailer"
)

// ProcessRenewalReminders sends staged and deduplicated reminders. It is safe
// to call hourly and from several API replicas because every stage is claimed
// in SQLite before SMTP delivery.
func ProcessRenewalReminders(db *sql.DB, cfg *config.Config, _ *mailer.Mailer, now time.Time) (int, error) {
	if db == nil || cfg == nil {
		return 0, fmt.Errorf("configuration des relances indisponible")
	}
	if _, err := customerPortalBaseURL(cfg.PublicWebsiteURL); err != nil {
		return 0, err
	}
	candidates, err := dbpkg.ListRenewalReminderCandidates(db, now.UTC())
	if err != nil {
		return 0, err
	}
	queued := 0
	var lastErr error
	for _, candidate := range candidates {
		claimed, claimErr := dbpkg.ClaimRenewalReminder(db, candidate, now.UTC())
		if claimErr != nil {
			lastErr = claimErr
			continue
		}
		if !claimed {
			continue
		}
		if queueErr := EnqueueRenewalReminderEmail(db, candidate); queueErr != nil {
			_ = dbpkg.FinishRenewalReminder(db, candidate, queueErr)
			lastErr = queueErr
			continue
		}
		queued++
	}
	return queued, lastErr
}

func customerPortalBaseURL(base string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("URL publique invalide")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/client/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
