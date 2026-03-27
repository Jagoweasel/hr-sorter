CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    slug TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS integrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER,
    platform TEXT NOT NULL,
    identifier TEXT NOT NULL,
    api_id INTEGER,
    api_hash TEXT,
    access_token TEXT,
    refresh_token TEXT,
    expires_at DATETIME,
    user_agent TEXT,
    status TEXT NOT NULL DEFAULT 'pending_auth',
    session_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    UNIQUE(platform, identifier)
);

CREATE TABLE IF NOT EXISTS contacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    integration_id INTEGER,
    platform TEXT NOT NULL DEFAULT 'tg',
    external_id TEXT UNIQUE NOT NULL,
    first_name TEXT,
    last_name TEXT,
    username TEXT,
    access_hash INTEGER DEFAULT 0,
    is_ignored BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    integration_id INTEGER,
    contact_id INTEGER,
    external_id TEXT,
    text TEXT,
    is_incoming BOOLEAN,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(integration_id, contact_id, external_id),
    FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE,
    FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sequences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER,
    company_name TEXT NOT NULL,
    vacancy_name TEXT NOT NULL,
    vacancy_link TEXT,
    status TEXT DEFAULT 'initial',
    rejection_reason TEXT,
    summary_comment TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sequence_contacts (
    sequence_id INTEGER,
    contact_id INTEGER,
    PRIMARY KEY (sequence_id, contact_id),
    FOREIGN KEY (sequence_id) REFERENCES sequences(id) ON DELETE CASCADE,
    FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS interview_stages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sequence_id INTEGER,
    name TEXT NOT NULL,
    stage_type TEXT,
    scheduled_at DATETIME,
    notes TEXT,
    is_completed BOOLEAN DEFAULT 0,
    order_index INTEGER,
    FOREIGN KEY (sequence_id) REFERENCES sequences(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tg_state (
    integration_id INTEGER PRIMARY KEY,
    pts INTEGER,
    qts INTEGER,
    seq INTEGER,
    date INTEGER,
    FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_filters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern TEXT NOT NULL,
    is_active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tg_channels (
    integration_id INTEGER,
    channel_id INTEGER,
    pts INTEGER,
    PRIMARY KEY (integration_id, channel_id),
    FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE
);
