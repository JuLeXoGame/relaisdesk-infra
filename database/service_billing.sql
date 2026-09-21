CREATE TABLE IF NOT EXISTS service_merchants (
 customer_id INTEGER PRIMARY KEY REFERENCES customers(id),
 enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
 account_id TEXT NOT NULL DEFAULT '',
 setup_started INTEGER NOT NULL,
 UNIQUE(account_id, customer_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS service_merchant_account ON service_merchants(account_id) WHERE account_id <> '';
CREATE TABLE IF NOT EXISTS service_rates (
 id TEXT PRIMARY KEY,
 customer_id INTEGER NOT NULL REFERENCES customers(id),
 label TEXT NOT NULL,
 mode TEXT NOT NULL CHECK(mode IN ('prepaid','hourly')),
 cents INTEGER NOT NULL CHECK(cents BETWEEN 50 AND 1000000),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN (0,1))
);
CREATE INDEX IF NOT EXISTS service_rates_customer ON service_rates(customer_id);
CREATE TABLE IF NOT EXISTS service_work (
 id TEXT PRIMARY KEY,
 customer_id INTEGER NOT NULL REFERENCES customers(id),
 license_id TEXT NOT NULL REFERENCES licences(license_id),
 member_id TEXT NOT NULL DEFAULT '',
 target_kind TEXT NOT NULL CHECK(target_kind IN ('device','code')),
 target_id TEXT NOT NULL,
 peer_id TEXT NOT NULL,
 label TEXT NOT NULL,
 mode TEXT NOT NULL CHECK(mode IN ('prepaid','hourly')),
 rate_cents INTEGER NOT NULL,
 account_id TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'prepared' CHECK(state IN ('prepared','open','finished','cancelled')),
 paid INTEGER NOT NULL DEFAULT 0,
 connected_ms INTEGER NOT NULL DEFAULT 0,
 amount_cents INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL,
 connection_id TEXT NOT NULL DEFAULT '',
 sequence INTEGER NOT NULL DEFAULT 0,
 cumulative_ms INTEGER NOT NULL DEFAULT 0,
 last_seen_ms INTEGER NOT NULL DEFAULT 0,
 connection_open INTEGER NOT NULL DEFAULT 0,
 checkout_id TEXT NOT NULL DEFAULT '',
 checkout_url TEXT NOT NULL DEFAULT '',
 checkout_started INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS service_work_owner ON service_work(customer_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS service_work_checkout ON service_work(checkout_id) WHERE checkout_id <> '';
CREATE TABLE IF NOT EXISTS service_terms_acceptances (
 customer_id INTEGER NOT NULL REFERENCES customers(id),
 version TEXT NOT NULL,
 document_hash TEXT NOT NULL,
 accepted_at INTEGER NOT NULL,
 PRIMARY KEY(customer_id, version)
);
CREATE TABLE IF NOT EXISTS service_work_closures (
 work_id TEXT PRIMARY KEY REFERENCES service_work(id) ON DELETE CASCADE,
 closed_at INTEGER NOT NULL
);
CREATE TRIGGER IF NOT EXISTS service_work_closed AFTER UPDATE OF state ON service_work
WHEN NEW.state IN ('finished','cancelled') AND OLD.state NOT IN ('finished','cancelled')
BEGIN
 INSERT OR IGNORE INTO service_work_closures(work_id,closed_at) VALUES(NEW.id,CAST(strftime('%s','now') AS INTEGER));
END;
-- Existing closed records start their retention period at migration, not before it.
INSERT OR IGNORE INTO service_work_closures(work_id,closed_at)
 SELECT id,CAST(strftime('%s','now') AS INTEGER) FROM service_work WHERE state IN ('finished','cancelled');
