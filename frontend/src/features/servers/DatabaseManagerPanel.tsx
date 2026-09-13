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
  ListWebsiteDBCredentials,
  SaveWebsiteDBCredential,
  ForgetWebsiteDBCredential,
  ListWebsiteDomainDatabases,
  LinkWebsiteDomainDatabase,
  UnlinkWebsiteDomainDatabase,
  GetWebsiteDBDockerAccessStatus,
  EnsureWebsiteDBDockerAccess,
} from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { MySQLExplorerModal } from './MySQLExplorerModal';
import { PGExplorerModal } from './PGExplorerModal';

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Utilitas provisioning MySQL/PostgreSQL ringan (database/user/grants) +
// Explore (koneksi driver asli, browse tabel/baris/query — lihat
// MySQLExplorerModal/PGExplorerModal), password disimpan LOKAL di vault
// mesin ini, TIDAK pernah ditulis ke server target.
//
// Dipakai di DUA tempat: (1) tab "Database" per-domain di Website (dengan
// `domain` diisi — tambahan: bisa menautkan database ke domain itu, murni
// kurasi lokal poinhost, tidak mengubah akses); (2) modul "Database"
// top-level per server (`domain` dikosongkan) — supaya bisa dipakai
// langsung tanpa perlu server itu punya domain/website apa pun dulu, sesuai
// temuan bahwa modul ini TERNYATA bukan benar-benar domain-scoped (lihat
// komentar di internal/modules/website/database.go): MySQL/PostgreSQL
// server-wide, Domain di request backend cuma breadcrumb UI.
export function DatabaseManagerPanel({ serverId, domain }: { serverId: string; domain?: string }) {
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
  const [saveCredential, setSaveCredential] = useState(true);

  const [credentials, setCredentials] = useState<website.DBCredentialInfo[]>([]);
  const [linkedDbs, setLinkedDbs] = useState<string[]>([]);
  const [linkingUser, setLinkingUser] = useState<website.DBUserInfo | null>(null);
  const [linkPassword, setLinkPassword] = useState('');
  const [exploreTarget, setExploreTarget] = useState<{ engine: 'mysql' | 'postgresql'; username: string; host: string } | null>(null);

  const [dockerAccess, setDockerAccess] = useState<website.DBDockerAccessStatus | null>(null);
  const [dockerAccessBusy, setDockerAccessBusy] = useState(false);

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
        setDockerAccess(await GetWebsiteDBDockerAccessStatus(serverId, engine));
      } else {
        setDockerAccess(null);
      }
      setCredentials(await ListWebsiteDBCredentials(serverId));
      if (domain) {
        const links = await ListWebsiteDomainDatabases(serverId, domain);
        setLinkedDbs(links.filter((l) => l.engine === engine).map((l) => l.database));
      } else {
        setLinkedDbs([]);
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
  }, [serverId, engine, domain]);

  function credentialFor(username: string, host: string) {
    const normalizedHost = engine === 'mysql' ? host || '%' : '-';
    return credentials.find((c) => c.engine === engine && c.username === username && c.host === normalizedHost);
  }

  function credentialLabel(username: string, host: string) {
    return engine === 'mysql' ? `${username}@${host || '%'}` : username;
  }

  async function handleLinkCredential() {
    if (!linkingUser || !linkPassword.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await SaveWebsiteDBCredential(
        new website.SaveDBCredentialRequest({
          serverId, engine, username: linkingUser.username, host: linkingUser.host, password: linkPassword, verify: true,
        }),
      );
      setLinkingUser(null);
      setLinkPassword('');
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleForgetCredential(username: string, host: string) {
    if (!confirm(`Lupakan password tersimpan untuk ${credentialLabel(username, host)}?`)) return;
    setBusy(true);
    try {
      await ForgetWebsiteDBCredential(serverId, engine, username, host);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleToggleDomainLink(dbName: string) {
    if (!domain) return;
    setBusy(true);
    setError(null);
    try {
      if (linkedDbs.includes(dbName)) {
        await UnlinkWebsiteDomainDatabase(serverId, domain, engine, dbName);
      } else {
        await LinkWebsiteDomainDatabase(serverId, domain, engine, dbName);
      }
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

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

  async function handleEnsureDockerAccess() {
    setDockerAccessBusy(true);
    setError(null);
    try {
      setDockerAccess(await EnsureWebsiteDBDockerAccess(serverId, engine));
    } catch (e) {
      setError(String(e));
    } finally {
      setDockerAccessBusy(false);
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
          saveCredential,
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
              <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: 0 }}>Akses dari Docker</h3>
            </div>
            {dockerAccess && (
              <div className="docker-engine">
                <p>
                  {dockerAccess.bindAllInterfaces ? (
                    <>✅ Container Docker di host ini sudah bisa connect ke {engine === 'mysql' ? 'MySQL/MariaDB' : 'PostgreSQL'} ini.</>
                  ) : (
                    <>
                      ⚠️ Container Docker di host ini <strong>belum bisa</strong> connect ke {engine === 'mysql' ? 'MySQL/MariaDB' : 'PostgreSQL'} ini —
                      masih hanya mendengarkan di 127.0.0.1 (loopback).
                    </>
                  )}
                  {dockerAccess.bindAllInterfaces && (
                    <>
                      {' '}
                      Firewall terdeteksi: <strong>{dockerAccess.firewallDetected || 'tidak ada'}</strong>
                      {dockerAccess.firewallDetected && dockerAccess.firewallDetected !== 'none' && (
                        <> ({dockerAccess.firewallRuleActive ? 'aturan poinhost aktif' : 'aturan poinhost belum ada'})</>
                      )}
                      .
                    </>
                  )}
                </p>
                {dockerAccess.message && <p className="overview__error">{dockerAccess.message}</p>}
                {(!dockerAccess.bindAllInterfaces || !dockerAccess.firewallRuleActive) && (
                  <button className="btn btn--sm btn--primary" disabled={dockerAccessBusy} onClick={() => void handleEnsureDockerAccess()}>
                    {dockerAccessBusy ? 'Menerapkan…' : '🐳 Aktifkan akses dari Docker'}
                  </button>
                )}
              </div>
            )}
          </section>

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
                    {domain && (
                      <td className="files-panel__row-actions">
                        <button
                          title={linkedDbs.includes(d.name) ? 'Tertaut ke situs ini — klik untuk lepas' : 'Tautkan database ini ke situs ini (kurasi lokal saja)'}
                          disabled={busy}
                          onClick={() => void handleToggleDomainLink(d.name)}
                        >
                          {linkedDbs.includes(d.name) ? '🔗 Tertaut' : '🔗 Tautkan'}
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
                {databases.length === 0 && (
                  <tr>
                    <td className="files-panel__empty">Belum ada database.</td>
                  </tr>
                )}
              </tbody>
            </table>
            {domain && linkedDbs.length > 0 && (
              <p className="chmod-path">Database yang dipakai situs ini: {linkedDbs.join(', ')}</p>
            )}
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
                <label className="form-check">
                  <input type="checkbox" checked={saveCredential} onChange={(e) => setSaveCredential(e.target.checked)} />
                  <span>Simpan password untuk Explore nanti (lokal, tidak dikirim ke server)</span>
                </label>
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
                {users.map((u) => {
                  const cred = credentialFor(u.username, u.host ?? '%');
                  return (
                    <tr key={u.username + (u.host ?? '')}>
                      <td>{u.username}</td>
                      {engine === 'mysql' && <td>{u.host}</td>}
                      <td className="files-panel__row-actions">
                        <button title="Terapkan ulang grants (privilege terpilih di atas, semua DB)" disabled={busy} onClick={() => void handleReapplyGrants(u)}>
                          🔑
                        </button>
                        {cred && (
                          <>
                            <button
                              title="Explore (browse tabel/baris)"
                              disabled={busy}
                              onClick={() => setExploreTarget({ engine, username: u.username, host: u.host ?? '%' })}
                            >
                              🔍
                            </button>
                            <button title="Lupakan password tersimpan" disabled={busy} onClick={() => void handleForgetCredential(u.username, u.host ?? '%')}>
                              ✖
                            </button>
                          </>
                        )}
                        {!cred && (
                          <button title="Hubungkan kredensial (simpan password untuk Explore)" disabled={busy} onClick={() => setLinkingUser(u)}>
                            🔗
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
                {users.length === 0 && (
                  <tr>
                    <td colSpan={3} className="files-panel__empty">
                      Belum ada user.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>

            {linkingUser && (
              <div className="docker-recreate">
                <p>
                  Hubungkan kredensial untuk <strong>{credentialLabel(linkingUser.username, linkingUser.host ?? '')}</strong> — password
                  diverifikasi dulu (coba konek langsung), lalu disimpan LOKAL di mesin ini (vault OS keychain / file terenkripsi), tidak pernah
                  dikirim ke server.
                </p>
                <div className="docker-recreate__row">
                  <input type="password" placeholder="password user tersebut" value={linkPassword} onChange={(e) => setLinkPassword(e.target.value)} />
                  <button className="btn btn--sm btn--primary" disabled={busy || !linkPassword.trim()} onClick={() => void handleLinkCredential()}>
                    Verifikasi & Simpan
                  </button>
                  <button className="btn btn--sm btn--ghost" onClick={() => setLinkingUser(null)}>
                    Batal
                  </button>
                </div>
              </div>
            )}
          </section>
        </>
      )}

      {exploreTarget && exploreTarget.engine === 'mysql' && (
        <MySQLExplorerModal
          serverId={serverId}
          username={exploreTarget.username}
          host={exploreTarget.host}
          onClose={() => setExploreTarget(null)}
        />
      )}
      {exploreTarget && exploreTarget.engine === 'postgresql' && (
        <PGExplorerModal serverId={serverId} username={exploreTarget.username} onClose={() => setExploreTarget(null)} />
      )}
    </div>
  );
}
