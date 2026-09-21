CREATE TABLE IF NOT EXISTS customers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id TEXT NOT NULL UNIQUE,
    customer_type TEXT NOT NULL DEFAULT 'business',
    display_name TEXT,
    billing_email TEXT NOT NULL COLLATE NOCASE UNIQUE,
    renewal_reminders_enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS customer_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER NOT NULL,
    email TEXT NOT NULL COLLATE NOCASE,
    role TEXT NOT NULL DEFAULT 'owner',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(customer_id, email),
    UNIQUE(email),
    FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS licences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER,
    license_id TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL,
    license_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    max_connections INTEGER NOT NULL DEFAULT 1,
    current_connections INTEGER NOT NULL DEFAULT 0,
    last_connection_at DATETIME,
    notes TEXT,
    revoked_at DATETIME,
    revoke_reason TEXT,
    FOREIGN KEY (customer_id) REFERENCES customers(id)
);

CREATE TABLE IF NOT EXISTS connections_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    license_id TEXT NOT NULL,
    client_ip TEXT NOT NULL,
    connected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    disconnected_at DATETIME,
    duration_seconds INTEGER,
    bytes_transferred INTEGER DEFAULT 0,
    FOREIGN KEY (license_id) REFERENCES licences(license_id)
);

CREATE TABLE IF NOT EXISTS server_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_key TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_active INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS viewer_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    technician_license_id TEXT NOT NULL,
    client_email TEXT,
    client_rustdesk_id TEXT DEFAULT '',
    network_device_public_key TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    is_active INTEGER NOT NULL DEFAULT 1,
    max_connections INTEGER NOT NULL DEFAULT 1,
    current_connections INTEGER NOT NULL DEFAULT 0,
    first_client_ip TEXT,
    last_connection_at DATETIME,
    revoked_reason TEXT,
    FOREIGN KEY (technician_license_id) REFERENCES licences(license_id)
);

CREATE TABLE IF NOT EXISTS technician_sessions (
    token TEXT PRIMARY KEY,
    license_id TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME,
    FOREIGN KEY (license_id) REFERENCES licences(license_id)
);

CREATE TABLE IF NOT EXISTS team_members (
    member_id TEXT PRIMARY KEY,
    license_id TEXT NOT NULL REFERENCES licences(license_id),
    owner_customer_id INTEGER NOT NULL REFERENCES customers(id),
    email TEXT NOT NULL COLLATE NOCASE,
    member_customer_id INTEGER REFERENCES customers(id),
    status TEXT NOT NULL CHECK(status IN ('invited','active','revoked')),
    invite_hash TEXT UNIQUE,
    invite_expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    accepted_at DATETIME,
    UNIQUE(license_id,email)
);

CREATE TABLE IF NOT EXISTS team_folder_grants (
    member_id TEXT NOT NULL REFERENCES team_members(member_id) ON DELETE CASCADE,
    folder_id TEXT NOT NULL REFERENCES device_folders(folder_id) ON DELETE CASCADE,
    PRIMARY KEY(member_id,folder_id)
);

CREATE TABLE IF NOT EXISTS admin_sessions (
    token_hash TEXT PRIMARY KEY,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME
);

CREATE TABLE IF NOT EXISTS security_alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    code TEXT,
    first_ip TEXT,
    second_ip TEXT,
    message TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS auth_bans (
    ip TEXT PRIMARY KEY,
    failed_count INTEGER NOT NULL DEFAULT 0,
    banned_until DATETIME NOT NULL,
    last_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reason TEXT NOT NULL DEFAULT 'brute_force'
);

CREATE TABLE IF NOT EXISTS orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER,
    order_id TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL,
    plan TEXT NOT NULL,
    technicians INTEGER NOT NULL DEFAULT 1,
    price REAL NOT NULL,
    payment_method TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    stripe_session_id TEXT,
    license_id TEXT,
    billing_name TEXT,
    billing_address TEXT,
    billing_postal_code TEXT,
    billing_city TEXT,
    billing_country TEXT DEFAULT 'France',
    billing_siret TEXT,
    invoice_number TEXT,
    customer_type TEXT NOT NULL DEFAULT 'business',
    terms_version TEXT NOT NULL DEFAULT 'legacy',
    terms_accepted_at DATETIME,
    immediate_performance_requested INTEGER NOT NULL DEFAULT 0,
	license_email_sent_at DATETIME,
    invoice_email_sent_at DATETIME,
    order_kind TEXT NOT NULL DEFAULT 'initial',
    renewal_license_id TEXT,
    billing_cycle TEXT NOT NULL DEFAULT 'monthly',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    paid_at DATETIME,
    expires_at DATETIME,
    notes TEXT,
    FOREIGN KEY (license_id) REFERENCES licences(license_id),
    FOREIGN KEY (renewal_license_id) REFERENCES licences(license_id),
    FOREIGN KEY (customer_id) REFERENCES customers(id)
);

CREATE TABLE IF NOT EXISTS invoices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER,
    invoice_number TEXT NOT NULL UNIQUE,
    order_id TEXT,
    customer_email TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    customer_address TEXT,
    customer_postal_code TEXT,
    customer_city TEXT,
    customer_country TEXT DEFAULT 'France',
    customer_siret TEXT,
    plan TEXT NOT NULL,
    technicians INTEGER NOT NULL DEFAULT 1,
    amount_ht REAL NOT NULL,
    amount_tva REAL NOT NULL DEFAULT 0.0,
    amount_ttc REAL NOT NULL,
    status TEXT NOT NULL DEFAULT 'paid',
    pdf_path TEXT NOT NULL,
    is_manual INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    FOREIGN KEY (order_id) REFERENCES orders(order_id),
    FOREIGN KEY (customer_id) REFERENCES customers(id)
);

CREATE TABLE IF NOT EXISTS withdrawal_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL UNIQUE,
    order_id TEXT NOT NULL,
    email TEXT NOT NULL,
    customer_name TEXT,
    requested_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'received',
    processed_at DATETIME,
    customer_email_sent_at DATETIME,
    admin_email_sent_at DATETIME,
    notes TEXT,
    FOREIGN KEY (order_id) REFERENCES orders(order_id)
);

CREATE TABLE IF NOT EXISTS customer_accounts (
    email TEXT PRIMARY KEY COLLATE NOCASE,
    customer_id INTEGER,
    password_hash TEXT,
    renewal_reminders_enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME
);

CREATE TABLE IF NOT EXISTS customer_login_tokens (
    token_hash TEXT PRIMARY KEY,
    email TEXT NOT NULL COLLATE NOCASE,
    customer_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    consumed_at DATETIME,
    FOREIGN KEY (email) REFERENCES customer_accounts(email)
);

CREATE TABLE IF NOT EXISTS customer_sessions (
    token_hash TEXT PRIMARY KEY,
    email TEXT NOT NULL COLLATE NOCASE,
    customer_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME NOT NULL,
    FOREIGN KEY (email) REFERENCES customer_accounts(email)
);

CREATE TABLE IF NOT EXISTS renewal_reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    license_id TEXT NOT NULL,
    reminder_type TEXT NOT NULL,
    cycle_expires_at DATETIME NOT NULL,
    claimed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at DATETIME,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    UNIQUE (license_id, reminder_type, cycle_expires_at),
    FOREIGN KEY (license_id) REFERENCES licences(license_id)
);

CREATE TABLE IF NOT EXISTS interventions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    intervention_id TEXT NOT NULL UNIQUE,
    license_id TEXT NOT NULL,
    viewer_code_id INTEGER,
    client_reference TEXT,
    title TEXT NOT NULL DEFAULT 'Assistance à distance',
    status TEXT NOT NULL DEFAULT 'planned',
    started_at DATETIME,
    ended_at DATETIME,
    duration_minutes INTEGER,
    summary TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (license_id) REFERENCES licences(license_id),
    FOREIGN KEY (viewer_code_id) REFERENCES viewer_codes(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    unique_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 12,
    available_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at DATETIME,
    completed_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Free trials are separate from paid orders: a setup or a zero-euro invoice
-- must never be treated as a paid order. Amounts and consent are snapshotted.
CREATE TABLE IF NOT EXISTS trial_applications (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL COLLATE NOCASE,
    customer_id INTEGER REFERENCES customers(id),
    plan TEXT NOT NULL,
    technicians INTEGER NOT NULL CHECK(technicians BETWEEN 1 AND 500),
    price_cents INTEGER NOT NULL CHECK(price_cents > 0),
    billing_cycle TEXT NOT NULL CHECK(billing_cycle IN ('monthly', 'annual')),
    billing_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    token_hash TEXT,
    token_expires_at INTEGER,
    verified_at INTEGER,
    state TEXT NOT NULL DEFAULT 'pending',
    stripe_customer_id TEXT,
    checkout_session_id TEXT UNIQUE,
    subscription_id TEXT UNIQUE,
    license_id TEXT UNIQUE REFERENCES licences(license_id),
    trial_start INTEGER NOT NULL DEFAULT 0,
    trial_end INTEGER NOT NULL DEFAULT 0,
    paid_through INTEGER NOT NULL DEFAULT 0,
    stripe_status TEXT NOT NULL DEFAULT '',
    cancel_at_period_end INTEGER NOT NULL DEFAULT 0,
    cancel_requested_at INTEGER,
    ended_at INTEGER,
    reminder_sent_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_trial_email ON trial_applications(email);
CREATE INDEX IF NOT EXISTS idx_trial_customer ON trial_applications(customer_id);
CREATE TABLE IF NOT EXISTS trial_claims (
    kind TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    application_id TEXT NOT NULL REFERENCES trial_applications(id),
    created_at INTEGER NOT NULL,
    PRIMARY KEY(kind, key_hash)
);
CREATE TABLE IF NOT EXISTS trial_hmac_check (
    singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
    digest TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS subscription_payments (
    stripe_invoice_id TEXT PRIMARY KEY,
    application_id TEXT NOT NULL REFERENCES trial_applications(id),
    order_id TEXT NOT NULL UNIQUE REFERENCES orders(order_id),
    period_start INTEGER NOT NULL,
    period_end INTEGER NOT NULL
);

-- A trial contract exists before a paid order. Keep its withdrawal evidence
-- separate, with one idempotent notice per contract and durable processing.
CREATE TABLE IF NOT EXISTS trial_withdrawals (
    application_id TEXT PRIMARY KEY REFERENCES trial_applications(id),
    request_id TEXT NOT NULL UNIQUE,
    requested_at INTEGER NOT NULL,
    customer_name TEXT NOT NULL DEFAULT '',
    immediate INTEGER NOT NULL DEFAULT 0,
    processed_at INTEGER,
    customer_email_sent_at INTEGER,
    admin_email_sent_at INTEGER,
    status TEXT NOT NULL DEFAULT 'received'
);
CREATE TABLE IF NOT EXISTS subscription_renewal_notices (
    application_id TEXT NOT NULL REFERENCES trial_applications(id),
    period_end INTEGER NOT NULL,
    sent_at INTEGER NOT NULL,
    notice_text TEXT NOT NULL,
    PRIMARY KEY(application_id, period_end)
);

CREATE TABLE IF NOT EXISTS billing_environment (
    singleton INTEGER PRIMARY KEY CHECK(singleton=1),
    mode TEXT NOT NULL CHECK(mode IN ('test','live'))
);

CREATE TABLE IF NOT EXISTS devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL UNIQUE,
    customer_id INTEGER,
    license_id TEXT,
    permanent_code TEXT NOT NULL UNIQUE,
    rustdesk_id TEXT NOT NULL DEFAULT '',
    device_public_key TEXT NOT NULL DEFAULT '',
    enrollment_version INTEGER NOT NULL DEFAULT 0,
    folder_id TEXT NOT NULL DEFAULT '',
    alias TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL DEFAULT '',
    os TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'offline',
    last_seen_at DATETIME,
    last_ip TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    is_active INTEGER NOT NULL DEFAULT 1,
    mac_address TEXT NOT NULL DEFAULT '',
    subnet_broadcast TEXT NOT NULL DEFAULT '',
    agent_version TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(id),
    FOREIGN KEY (license_id) REFERENCES licences(license_id)
);

CREATE TABLE IF NOT EXISTS device_folders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    folder_id TEXT NOT NULL UNIQUE,
    customer_id INTEGER,
    license_id TEXT,
    parent_folder_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(id),
    FOREIGN KEY (license_id) REFERENCES licences(license_id)
);
CREATE INDEX IF NOT EXISTS idx_device_folders_customer ON device_folders(customer_id);
CREATE INDEX IF NOT EXISTS idx_device_folders_license ON device_folders(license_id);
CREATE INDEX IF NOT EXISTS idx_device_folders_parent ON device_folders(parent_folder_id);

CREATE TABLE IF NOT EXISTS device_auth_nonces (
    device_id TEXT NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
    nonce TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY (device_id, nonce)
);
CREATE INDEX IF NOT EXISTS idx_device_auth_nonce_expiry ON device_auth_nonces(expires_at);

CREATE TABLE IF NOT EXISTS device_wake_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_device_id TEXT NOT NULL,
    license_id TEXT NOT NULL,
    mac_address TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    dispatched_at DATETIME,
    FOREIGN KEY (target_device_id) REFERENCES devices(device_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_device_wake_license ON device_wake_requests(license_id, status);

CREATE TABLE IF NOT EXISTS device_update_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    target_version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    dispatched_at DATETIME,
    FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_device_update_req ON device_update_requests(device_id, status);

