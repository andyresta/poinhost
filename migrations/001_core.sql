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
    use_sudo      INTEGER NOT NULL DEFAULT 0,
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

-- website_db_credentials menyimpan METADATA kredensial database MySQL yang
-- dikelola dari fitur Explore — password ASLINYA disimpan di secrets vault
-- lokal (OS keychain, fallback file AES-256-GCM di ~/.poinhost/secrets.json),
-- bukan di kolom tabel ini, dan TIDAK PERNAH ditulis balik ke server target
-- (beda dari homepoin yang menaruh file JSON terenkripsi DI server yang
-- dikelola). Baris di sini cuma menandai "kredensial user X@host di server Y
-- sudah tersimpan", supaya UI bisa menampilkan daftarnya tanpa perlu
-- mengorek isi vault (yang sama sekali tidak mendukung listing di beberapa
-- backend OS keychain).
CREATE TABLE IF NOT EXISTS website_db_credentials (
    id            TEXT PRIMARY KEY,
    server_id     TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    engine        TEXT NOT NULL DEFAULT 'mysql',
    username      TEXT NOT NULL,
    host          TEXT NOT NULL DEFAULT '%',
    verified_at   TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(server_id, engine, username, host)
);

-- website_domain_databases menautkan database ke domain SECARA ORGANISATORIS
-- SAJA (kurasi lokal poinhost) — MySQL sendiri tidak mengenal scoping per
-- domain (grants tetap berlaku server-wide seperti biasa), jadi tabel ini
-- TIDAK mengubah akses apa pun, cuma supaya tab Database di suatu domain bisa
-- menampilkan "database yang dipakai situs ini". Ini yang bikin fitur
-- Database poinhost benar-benar terelasi dengan Website, beda dari homepoin
-- yang field Domain-nya sekadar hiasan UI tak terpakai di backend.
CREATE TABLE IF NOT EXISTS website_domain_databases (
    id            TEXT PRIMARY KEY,
    server_id     TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    domain        TEXT NOT NULL,
    engine        TEXT NOT NULL DEFAULT 'mysql',
    db_name       TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(server_id, domain, engine, db_name)
);

CREATE INDEX IF NOT EXISTS idx_website_domain_databases_domain
    ON website_domain_databases(server_id, domain);
