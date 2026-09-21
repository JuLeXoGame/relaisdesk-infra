CREATE INDEX IF NOT EXISTS idx_licences_license_id ON licences(license_id);
CREATE INDEX IF NOT EXISTS idx_licences_license_key ON licences(license_key);
CREATE INDEX IF NOT EXISTS idx_licences_status ON licences(status);
CREATE INDEX IF NOT EXISTS idx_licences_email ON licences(email);
CREATE INDEX IF NOT EXISTS idx_licences_customer ON licences(customer_id);
CREATE INDEX IF NOT EXISTS idx_connections_license ON connections_log(license_id);

CREATE INDEX IF NOT EXISTS idx_viewer_codes_code ON viewer_codes(code);
CREATE INDEX IF NOT EXISTS idx_viewer_codes_technician ON viewer_codes(technician_license_id);
CREATE INDEX IF NOT EXISTS idx_viewer_codes_expires ON viewer_codes(expires_at);

CREATE INDEX IF NOT EXISTS idx_tech_sessions_license ON technician_sessions(license_id);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires ON admin_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_security_alerts_type ON security_alerts(type);
CREATE INDEX IF NOT EXISTS idx_security_alerts_code ON security_alerts(code);
CREATE INDEX IF NOT EXISTS idx_security_alerts_created_at ON security_alerts(created_at);

CREATE INDEX IF NOT EXISTS idx_orders_order_id ON orders(order_id);
CREATE INDEX IF NOT EXISTS idx_orders_email ON orders(email);
CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_stripe_session ON orders(stripe_session_id);
CREATE INDEX IF NOT EXISTS idx_orders_invoice ON orders(invoice_number);
CREATE INDEX IF NOT EXISTS idx_orders_renewal_license ON orders(renewal_license_id);
CREATE INDEX IF NOT EXISTS idx_orders_paid_license_plan ON orders(license_id, id DESC) WHERE status='paid';
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_open_renewal ON orders(renewal_license_id)
    WHERE order_kind = 'renewal' AND status IN ('pending', 'processing');

CREATE INDEX IF NOT EXISTS idx_invoices_number ON invoices(invoice_number);
CREATE INDEX IF NOT EXISTS idx_invoices_order_id ON invoices(order_id);
CREATE INDEX IF NOT EXISTS idx_invoices_email ON invoices(customer_email);
CREATE INDEX IF NOT EXISTS idx_invoices_customer ON invoices(customer_id);
CREATE INDEX IF NOT EXISTS idx_invoices_created_at ON invoices(created_at);

CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_order ON withdrawal_requests(order_id);
CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_status ON withdrawal_requests(status);
CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_requested_at ON withdrawal_requests(requested_at);

CREATE INDEX IF NOT EXISTS idx_customer_login_tokens_email ON customer_login_tokens(email);
CREATE INDEX IF NOT EXISTS idx_customer_login_tokens_expires ON customer_login_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_email ON customer_sessions(email);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_expires ON customer_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_renewal_reminders_license ON renewal_reminders(license_id);
CREATE INDEX IF NOT EXISTS idx_interventions_license ON interventions(license_id);
CREATE INDEX IF NOT EXISTS idx_interventions_viewer_code ON interventions(viewer_code_id);
CREATE INDEX IF NOT EXISTS idx_interventions_started ON interventions(started_at);
CREATE INDEX IF NOT EXISTS idx_jobs_ready ON jobs(status, available_at);
CREATE INDEX IF NOT EXISTS idx_auth_bans_banned_until ON auth_bans(banned_until);

CREATE INDEX IF NOT EXISTS idx_devices_customer ON devices(customer_id);
CREATE INDEX IF NOT EXISTS idx_devices_license ON devices(license_id);
CREATE INDEX IF NOT EXISTS idx_devices_permanent_code ON devices(permanent_code);
CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(status);
CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_devices_folder ON devices(folder_id);
CREATE INDEX IF NOT EXISTS idx_team_members_owner ON team_members(owner_customer_id,license_id,status);
CREATE INDEX IF NOT EXISTS idx_team_members_email ON team_members(email,status);
CREATE INDEX IF NOT EXISTS idx_team_sessions_member ON technician_sessions(team_member_id);
