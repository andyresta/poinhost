import { useEffect, useState } from 'react';
import { useTabsStore } from '../../store/tabs';
import { TestServerConnection } from '../../../wailsjs/go/main/App';
import { servers } from '../../../wailsjs/go/models';

const emptyForm: servers.SaveServerRequest = new servers.SaveServerRequest({
  name: '',
  host: '',
  port: 22,
  username: '',
  authType: 'key',
  keyPath: '',
  password: '',
  tags: [],
  color: '#6366f1',
  notes: '',
});

// Panel kiri: daftar server terdaftar + form tambah server. Klik satu
// server akan memfokuskan tab yang SUDAH terbuka untuk server itu kalau
// ada (supaya tidak sengaja menumpuk tab duplikat setiap klik), atau
// membuka tab baru kalau belum ada satupun.
export function ServersPage() {
  const { servers: list, tabs, loadServers, openTab, setActiveTab, saveServer, deleteServer } =
    useTabsStore();
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<servers.SaveServerRequest>(emptyForm);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void loadServers();
  }, [loadServers]);

  function openOrFocus(server: servers.Server) {
    const existing = tabs.find((t) => t.serverId === server.id);
    if (existing) {
      setActiveTab(existing.id);
      return;
    }
    void openTab(server.id, server.name);
  }

  async function handleTest() {
    setBusy(true);
    setTestResult(null);
    try {
      const res = await TestServerConnection(form);
      setTestResult(
        res.status === 'ok'
          ? `OK — ${res.latency}`
          : `${res.status}${res.message ? `: ${res.message}` : ''}`,
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleSave() {
    setBusy(true);
    try {
      await saveServer(form);
      setForm(emptyForm);
      setShowForm(false);
      setTestResult(null);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="servers-page">
      <div className="servers-page__header">
        <h1>Servers</h1>
        <button onClick={() => setShowForm((v) => !v)}>{showForm ? 'Batal' : '+ Tambah'}</button>
      </div>

      {showForm && (
        <div className="servers-page__form">
          <input
            placeholder="Nama"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <input
            placeholder="Host"
            value={form.host}
            onChange={(e) => setForm({ ...form, host: e.target.value })}
          />
          <input
            placeholder="Port"
            type="number"
            value={form.port}
            onChange={(e) => setForm({ ...form, port: Number(e.target.value) })}
          />
          <input
            placeholder="Username"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
          />
          <select
            value={form.authType}
            onChange={(e) => setForm({ ...form, authType: e.target.value })}
          >
            <option value="key">SSH Key</option>
            <option value="password">Password</option>
          </select>
          {form.authType === 'key' ? (
            <input
              placeholder="Key path (kosong = ~/.ssh/id_ed25519)"
              value={form.keyPath}
              onChange={(e) => setForm({ ...form, keyPath: e.target.value })}
            />
          ) : (
            <input
              placeholder="Password"
              type="password"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
            />
          )}
          <div className="servers-page__form-actions">
            <button disabled={busy} onClick={() => void handleTest()}>
              Test Koneksi
            </button>
            <button disabled={busy} onClick={() => void handleSave()}>
              Simpan
            </button>
          </div>
          {testResult && <div className="servers-page__test-result">{testResult}</div>}
        </div>
      )}

      <ul className="servers-page__list">
        {list.map((server) => (
          <li key={server.id} className="servers-page__item" onClick={() => openOrFocus(server)}>
            <span className="servers-page__dot" style={{ backgroundColor: server.color }} />
            <div className="servers-page__meta">
              <div className="servers-page__name">{server.name}</div>
              <div className="servers-page__host">
                {server.username}@{server.host}:{server.port}
              </div>
            </div>
            <button
              className="servers-page__delete"
              title="Hapus"
              onClick={(e) => {
                e.stopPropagation();
                void deleteServer(server.id);
              }}
            >
              🗑
            </button>
          </li>
        ))}
        {list.length === 0 && !showForm && (
          <li className="servers-page__empty">Belum ada server. Klik "+ Tambah".</li>
        )}
      </ul>
    </div>
  );
}
