package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"api/config"
	"api/mailer"
	dbpkg "database"
)

const jobCartReminder = "cart_reminder_email"

type cartReminderPayload struct {
	OrderID string `json:"order_id"`
	Round   int    `json:"round"`
}

// QueueCartReminders enqueues cart recovery emails for unpaid Stripe orders:
// round 1 at 24h old, round 2 ("last call") at 40h old, both before the 48h
// order expiry. Dedup keys make the hourly pass idempotent.
func QueueCartReminders(db *sql.DB, now time.Time) (int, error) {
	now = now.UTC()
	rounds := []struct {
		round int
		from  time.Time
		to    time.Time
	}{
		{1, now.Add(-40 * time.Hour), now.Add(-24 * time.Hour)},
		{2, now.Add(-48 * time.Hour), now.Add(-40 * time.Hour)},
	}
	queued := 0
	for _, r := range rounds {
		orders, err := dbpkg.ListAbandonedCarts(db, r.from, r.to)
		if err != nil {
			return queued, err
		}
		for _, order := range orders {
			if strings.TrimSpace(order.Email) == "" {
				continue
			}
			ok, err := enqueueJSONJob(db, jobCartReminder,
				cartReminderPayload{OrderID: order.OrderID, Round: r.round},
				fmt.Sprintf("cart-reminder-r%d:%s", r.round, order.OrderID), 8)
			if err != nil {
				return queued, err
			}
			if ok {
				queued++
			}
		}
	}
	return queued, nil
}

func processCartReminderEmail(db *sql.DB, cfg *config.Config, mail *mailer.Mailer, job *dbpkg.Job) error {
	var payload cartReminderPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return err
	}
	order, err := dbpkg.GetOrderByID(db, payload.OrderID)
	if err != nil {
		return err
	}
	// Paid, cancelled or expired since enqueue: nothing to recover.
	if order.Status != "pending" && order.Status != "processing" {
		return nil
	}
	if strings.TrimSpace(order.Email) == "" {
		return nil
	}
	expires := ""
	if order.ExpiresAt != nil {
		expires = order.ExpiresAt.UTC().Format("02/01/2006 à 15:04")
	}
	resumeURL := strings.TrimRight(cfg.PublicWebsiteURL, "/") + "/#pricing"
	return mail.SendCartReminderEmail(order.Email, order.OrderID, order.Plan, order.Price, expires, resumeURL, payload.Round)
}
