import { useEffect, useRef, useState } from 'react';
import {
  GetWebsiteDBStatus,
  StartWebsiteDB,
  StreamWebsiteInstall,
  StopDockerStream,
  ListWebsiteDatabases,
  CreateWebsiteDatabase,
  ListWebsiteDatabaseUsers,
  CreateWebsiteDatabaseUser,
  SetWebsiteDatabaseGrants,
  GetWebsiteDBPrivileges,
} from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Tab Database — TERNYATA (sama seperti homepoin) bukan benar-benar
// domain-scoped: ini utilitas provisioning MySQL/PostgreSQL ringan
// (database/user/grants), kebetulan bisa dibuka dari tab satu domain untuk
// kenyamanan, terpisah total dari database browser server-wide (di luar
// scope port ini).
export function DomainDatabaseTab({ serverId }: { serverId: string; domain: string }) {
  const [engine, setEngine] = useState<'mysql' | 'postgresql'>('mysql');
  const [status, setStatus] = useState<website.DBEngineStatus | null>(null);
  const [databases, setDatabases] = useState<website.DBDatabaseInfo[]>([]);
  const [users, setUsers] = useState<website.DBUserInfo[]>([]);
  const [privileges, setPrivileges] = useState<website.DBPrivilegeOption[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [lines, setLines] = useState<string[]>([]);
  const streamIdRef = useRef<string | null>(null);

  const [newDbName, setNewDbName] = useState('');
  const [showUserForm, setShowUserForm] = useState(false);
  const [newUser, setNewUser] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newHost, setNewHost] = useState('%');
  const [allDbs, setAllDbs] = useState(true);
  const [selectedDbs, setSelectedDbs] = useState<string[]>([]);
  const [selectedPrivs, setSelectedPrivs] = useState<string[]>(['read', 'write']);

  useEffect(() => {
    GetWebsiteDBPrivileges().then(setPrivileges).catch(() => setPrivileges([]));
  }, []);

  async function load() {
    setError(null);
    try {
      const st = await GetWebsiteDBStatus(serverId, engine);
      setStatus(st);
      if (st.installed && st.active) {
        setDatabases(await ListWebsiteDatabases(serverId, engine));
        setUsers(await ListWebsiteDatabaseUsers(serverId, engine));
      }
    } catch (e) {
      setError(String(e));
    }
  }

  useEffect(() => {
    void load();
    return () => {
      if (streamIdRef.current) void StopDockerStream(streamIdRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, engine]);

  async function handleStart() {
    setBusy(true);
    setError(null);
    try {
      await StartWebsiteDB(serverId, engine);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleInstall() {
    setLines([]);
    setError(null);
    setInstalling(true);
    const streamId = await StreamWebsiteInstall(serverId, engine, '');
    streamIdRef.current = streamId;
    const unsub = EventsOn(`website:install:${streamId}`, (evt: StreamLineEvent) => {
      if (evt.type === 'line' && evt.line) setLines((l) => [...l, evt.line as string]);
      else if (evt.type === 'error') {
        setError(evt.message ?? 'Instalasi gagal');
        setInstalling(false);
        streamIdRef.current = null;
        unsub();
      } else if (evt.type === 'end') {
        setInstalling(false);
        streamIdRef.current = null;
        unsub();
        void load();
      }
    });
  }

  async function handleCreateDb() {
    if (!newDbName.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await CreateWebsiteDatabase(new website.DBCreateDatabaseRequest({ serverId, engine, name: newDbName.trim() }));
      setNewDbName('');
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleCreateUser() {
    if (!newUser.trim() || !newPassword.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await CreateWebsiteDatabaseUser(
        new website.DBCreateUserRequest({
          serverId,
          engine,
          username: newUser.trim(),
          password: newPassword,
          host: engine === 'mysql' ? newHost.trim() || '%' : undefined,
          allDbs: allDbs,
          databases: allDbs ? [] : selectedDbs,
          privileges: selectedPrivs,
        }),
      );
      setNewUser('');
      setNewPassword('');
      setShowUserForm(false);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleReapplyGrants(user: website.DBUserInfo) {
    setBusy(true);
    setError(null);
    try {
      await SetWebsiteDatabaseGrants(
        new website.DBGrantsRequest({
          serverId,
          engine,
          username: user.username,
          host: user.host,
          allDbs: true,
          privileges: selectedPrivs,
        }),
      );
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  function togglePriv(key: string) {
    setSelectedPrivs((p) => (p.includes(key) ? p.filter((x) => x !== key) : [...p, key]));
  }

  function toggleDb(name: string) {
    setSelectedDbs((d) => (d.includes(name) ? d.filter((x) => x !== name) : [...d, name]));
  }

  return (
    <div>
      <div className="segmented" style={{ marginBottom: 10 }}>
        <button className={`segmented__item${engine === 'mysql' ? ' segmented__item--active' : ''}`} onClick={() => setEngine('mysql')}>
          MySQL / MariaDB
        </button>
        <button
          className={`segmented__item${engine === 'postgresql' ? ' segmented__item--active' : ''}`}
          onClick={() => setEngine('postgresql')}
        >
          PostgreSQL
        </button>
      </div>

      {error && <p className="overview__error">{error}</p>}
      {!status && <p className="workspace__placeholder">Memuat…</p>}

      {status && status.installed && !status.active && (
        <div className="docker-engine">
          <p>{engine === 'mysql' ? 'MySQL/MariaDB' : 'PostgreSQL'} sudah terpasang tapi tidak aktif.</p>
          <button className="btn btn--primary" disabled={busy} onClick={() => void handleStart()}>
            Jalankan
          </button>
        </div>
      )}

      {status && !status.installed && (
        <div className="docker-engine">
          <p>
            {engine === 'mysql' ? 'MySQL/MariaDB' : 'PostgreSQL'} belum terpasang{status.distroName ? ` (distro: ${status.distroName})` : ''}.
          </p>
          {!status.canInstall && <p className="overview__error">Distro server ini belum didukung instalasi otomatis.</p>}
          {status.canInstall && (
            <button className="btn btn--primary" disabled={installing} onClick={() => void handleInstall()}>
              {installing ? 'Menginstal…' : `Install ${engine === 'mysql' ? 'MariaDB' : 'PostgreSQL'}`}
            </button>
          )}
          {lines.length > 0 && <pre className="docker-engine__log">{lines.join('\n')}</pre>}
        </div>
      )}

      {status && status.installed && status.active && (
        <>
          <section style={{ marginBottom: 16 }}>
            <div className="files-panel__toolbar">
              <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: 0 }}>Database</h3>
              <input placeholder="nama_database" value={newDbName} onChange={(e) => setNewDbName(e.target.value)} style={{ marginLeft: 'auto' }} />
              <button className="btn btn--sm btn--primary" disabled={busy || !newDbName.trim()} onClick={() => void handleCreateDb()}>
                + Buat
              </button>
            </div>
            <table className="files-panel__table">
              <tbody>
                {databases.map((d) => (
                  <tr key={d.name}>
                    <td>{d.name}</td>
                  </tr>
                ))}
                {databases.length === 0 && (
                  <tr>
                    <td className="files-panel__empty">Belum ada database.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>

          <section>
            <div className="files-panel__toolbar">
              <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: 0 }}>User</h3>
              <button className="btn btn--sm btn--primary" style={{ marginLeft: 'auto' }} onClick={() => setShowUserForm((v) => !v)}>
                + User Baru
              </button>
            </div>

            {showUserForm && (
              <div className="docker-recreate">
                <div className="docker-recreate__row">
                  <input placeholder="username" value={newUser} onChange={(e) => setNewUser(e.target.value)} />
                  <input type="password" placeholder="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
                  {engine === 'mysql' && <input placeholder="host (%)" value={newHost} onChange={(e) => setNewHost(e.target.value)} style={{ maxWidth: 100 }} />}
                </div>
                <label className="form-check">
                  <input type="checkbox" checked={allDbs} onChange={(e) => setAllDbs(e.target.checked)} />
                  <span>Akses semua database</span>
                </label>
                {!allDbs && (
                  <div className="chip-row" style={{ display: 'flex', gap: 6, flexWrap: 'wrap', margin: '6px 0' }}>
                    {databases.map((d) => (
                      <label key={d.name} className="form-check" style={{ marginTop: 0 }}>
                        <input type="checkbox" checked={selectedDbs.includes(d.name)} onChange={() => toggleDb(d.name)} />
                        <span>{d.name}</span>
                      </label>
                    ))}
                  </div>
                )}
                <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', margin: '8px 0' }}>
                  {privileges.map((p) => (
                    <label key={p.key} className="form-check" style={{ marginTop: 0 }}>
                      <input type="checkbox" checked={selectedPrivs.includes(p.key)} onChange={() => togglePriv(p.key)} />
                      <span>{p.label}</span>
                    </label>
                  ))}
                </div>
                <button className="btn btn--sm btn--primary" disabled={busy || !newUser.trim() || !newPassword.trim()} onClick={() => void handleCreateUser()}>
                  Buat User
                </button>
              </div>
            )}

            <table className="files-panel__table">
              <thead>
                <tr>
                  <th>Username</th>
                  {engine === 'mysql' && <th>Host</th>}
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.username + (u.host ?? '')}>
                    <td>{u.username}</td>
                    {engine === 'mysql' && <td>{u.host}</td>}
                    <td className="files-panel__row-actions">
                      <button title="Terapkan ulang grants (privilege terpilih di atas, semua DB)" disabled={busy} onClick={() => void handleReapplyGrants(u)}>
                        🔑
                      </button>
                    </td>
                  </tr>
                ))}
                {users.length === 0 && (
                  <tr>
                    <td colSpan={3} className="files-panel__empty">
                      Belum ada user.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>
        </>
      )}
    </div>
  );
}
