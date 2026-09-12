-- Bootstrap schema poinhost (idempotent).
-- Dijalankan setiap startup; gunakan CREATE IF NOT EXISTS / INSERT OR IGNORE saja.

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS servers (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    host          TEXT NOT NULL,
    port          INTEGER NOT NULL DEFAULT 22,
    username      TEXT NOT NULL,
    auth_type     TEXT NOT NULL DEFAULT 'key',
    key_path      TEXT,
    password_enc  TEXT,
    tags          TEXT NOT NULL DEFAULT '[]',
    color         TEXT NOT NULL DEFAULT '#6366f1',
    notes         TEXT NOT NULL DEFAULT '',
    is_active     INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS app_settings (
    key           TEXT PRIMARY KEY,
    value         TEXT NOT NULL,
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO app_settings (key, value) VALUES
    ('app.theme',    'dark'),
    ('app.language', 'id');

CREATE TABLE IF NOT EXISTS activity_logs (
    id            TEXT PRIMARY KEY,
    server_id     TEXT REFERENCES servers(id) ON DELETE SET NULL,
    module        TEXT NOT NULL,
    action        TEXT NOT NULL,
    detail        TEXT,
    status        TEXT NOT NULL DEFAULT 'success',
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_activity_logs_server
    ON activity_logs(server_id, created_at DESC);

-- ui_tabs menyimpan layout tab terbuka terakhir (server_id + modul aktif +
-- posisi), supaya saat aplikasi ditutup lalu dibuka lagi, tab-tab yang
-- sedang dikelola bisa direstore persis seperti browser me-restore tab.
-- Ini murni bookkeeping UI — TIDAK menyimpan koneksi SSH apapun (koneksi
-- selalu dibuat ulang dari sshpool saat tab dibuka/direstore).
CREATE TABLE IF NOT EXISTS ui_tabs (
    id             TEXT PRIMARY KEY,
    server_id      TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    title          TEXT NOT NULL DEFAULT '',
    active_module  TEXT NOT NULL DEFAULT 'overview',
    position       INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ui_tabs_position ON ui_tabs(position ASC);
