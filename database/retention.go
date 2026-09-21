package database

import (
	"database/sql"
	"fmt"
	"time"
)

// RetentionPurgeResult reports records removed under the published retention policy.
type RetentionPurgeResult struct {
	ViewerCodes            int64
	ConnectionLogs         int64
	SecurityAlerts         int64
	UnpaidOrders           int64
	WithdrawalRecords      int64
	Interventions          int64
	CompletedJobs          int64
	AuthBans               int64
	TrialClaims            int64
	AbandonedTrials        int64
	TrialWithdrawalRecords int64
	SubscriptionNotices    int64
	ServiceWork            int64
	ServiceDrafts          int64
}

// PurgeExpiredOperationalData deletes records whose published retention period has elapsed.
func PurgeExpiredOperationalData(db *sql.DB, now time.Time) (RetentionPurgeResult, error) {
	var result RetentionPurgeResult
	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	closed, err := tx.Exec(`DELETE FROM service_work WHERE state IN ('finished','cancelled') AND id IN (SELECT work_id FROM service_work_closures WHERE closed_at < ?)`, now.AddDate(-5, 0, 0).Unix())
	if err != nil {
		return result, err
	}
	result.ServiceWork, _ = closed.RowsAffected()
	drafts, err := tx.Exec(`DELETE FROM service_work WHERE state='prepared' AND paid=0 AND checkout_started=0 AND checkout_id='' AND connected_ms=0 AND last_seen_ms=0 AND created_at < ?`, now.AddDate(0, 0, -90).Unix())
	if err != nil {
		return result, err
	}
	result.ServiceDrafts, _ = drafts.RowsAffected()

	claims, err := tx.Exec(`DELETE FROM trial_claims WHERE application_id IN (
		SELECT id FROM trial_applications WHERE
		(state='rejected' OR ended_at IS NOT NULL OR (state='provisioning' AND subscription_id IS NULL))
		AND MAX(COALESCE(ended_at,0),trial_end,paid_through,created_at) < ?
	)`, now.AddDate(-3, 0, 0).Unix())
	if err != nil {
		return result, err
	}
	result.TrialClaims, _ = claims.RowsAffected()
	for _, item := range []struct {
		table  string
		target *int64
	}{{"trial_withdrawals", &result.TrialWithdrawalRecords}, {"subscription_renewal_notices", &result.SubscriptionNotices}} {
		res, e := tx.Exec(`DELETE FROM `+item.table+` WHERE application_id IN (SELECT id FROM trial_applications WHERE ended_at IS NOT NULL AND ended_at < ?)`, now.AddDate(-5, 0, 0).Unix())
		if e != nil {
			return result, e
		}
		*item.target, _ = res.RowsAffected()
	}
	abandoned, err := tx.Exec(`DELETE FROM trial_applications WHERE state IN ('pending','verified','rejected')
		AND created_at < ? AND subscription_id IS NULL
		AND NOT EXISTS (SELECT 1 FROM trial_claims c WHERE c.application_id=trial_applications.id)
		AND NOT EXISTS (SELECT 1 FROM trial_withdrawals w WHERE w.application_id=trial_applications.id)`, now.AddDate(0, 0, -30).Unix())
	if err != nil {
		return result, err
	}
	result.AbandonedTrials, _ = abandoned.RowsAffected()

	queries := []struct {
		target *int64
		query  string
		cutoff time.Time
	}{
		{&result.AuthBans, `DELETE FROM auth_bans WHERE datetime(banned_until) < datetime(?)`, now.AddDate(0, 0, -7)},
		{&result.ViewerCodes, `DELETE FROM viewer_codes WHERE datetime(expires_at) < datetime(?)`, now.AddDate(0, 0, -30)},
		{&result.ConnectionLogs, `DELETE FROM connections_log WHERE datetime(connected_at) < datetime(?)`, now.AddDate(-1, 0, 0)},
		{&result.SecurityAlerts, `DELETE FROM security_alerts WHERE resolved_at IS NOT NULL AND datetime(resolved_at) < datetime(?)`, now.AddDate(-1, 0, 0)},
		{&result.WithdrawalRecords, `DELETE FROM withdrawal_requests WHERE datetime(requested_at) < datetime(?)`, now.AddDate(-5, 0, 0)},
		{&result.Interventions, `DELETE FROM interventions
			WHERE status IN ('completed', 'cancelled')
			  AND datetime(COALESCE(ended_at, updated_at)) < datetime(?)`, now.AddDate(-5, 0, 0)},
		{&result.CompletedJobs, `DELETE FROM jobs
			WHERE status IN ('done', 'dead') AND datetime(completed_at) < datetime(?)`, now.AddDate(0, 0, -30)},
		{&result.UnpaidOrders, `DELETE FROM orders
			WHERE status IN ('cancelled', 'pending')
			  AND datetime(expires_at) < datetime(?)
			  AND invoice_number IS NULL
			  AND NOT EXISTS (SELECT 1 FROM withdrawal_requests wr WHERE wr.order_id = orders.order_id)`, now.AddDate(0, -3, 0)},
	}

	for _, item := range queries {
		execResult, err := tx.Exec(item.query, item.cutoff.UTC().Format(time.RFC3339))
		if err != nil {
			return result, fmt.Errorf("purge des données opérationnelles: %w", err)
		}
		*item.target, err = execResult.RowsAffected()
		if err != nil {
			return result, fmt.Errorf("comptage de la purge: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("validation de la purge: %w", err)
	}
	return result, nil
}
