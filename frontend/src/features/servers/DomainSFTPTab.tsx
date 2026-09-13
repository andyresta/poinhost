import { useEffect, useState } from 'react';
import { RotateCw, Trash2 } from 'lucide-react';
import { ListWebsiteSFTPAccounts, CreateWebsiteSFTPAccount, DeleteWebsiteSFTPAccount } from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';

// Tab SFTP — akun Linux ter-chroot (bukan reuse user SSH panel) khusus
// untuk satu domain: chroot default ke folder induk document root, home
// login default ke /public_html. Dipakai kalau ingin memberi klien/developer
// akses upload file TANPA akses shell/SSH penuh ke server.
export function DomainSFTPTab({ serverId, domain }: { serverId: string; domain: string }) {
  const [list, setList] = useState<website.SFTPListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');

  async function load() {
    try {
      setList(await ListWebsiteSFTPAccounts(serverId, domain));
    } catch (e) {
      setError(String(e));
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, domain]);

  async function handleCreate() {
    if (!username.trim() || !password.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await CreateWebsiteSFTPAccount(new website.SFTPCreateAccountRequest({ serverId, domain, username: username.trim(), password }));
      setUsername('');
      setPassword('');
      setShowForm(false);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(user: string) {
    if (!confirm(`Hapus akun SFTP "${user}"? User Linux-nya akan ikut dihapus.`)) return;
    setBusy(true);
    setError(null);
    try {
      await DeleteWebsiteSFTPAccount(serverId, domain, user);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  if (!list) return <p className="workspace__placeholder">Memuat…</p>;

  return (
    <div>
      {error && <p className="overview__error">{error}</p>}
      <p className="chmod-path">Chroot default: {list.domainRoot.replace(/\/public_html$/, '')}</p>

      <div className="files-panel__toolbar">
        <button className="btn btn--sm" disabled={busy} onClick={() => void load()}>
          <RotateCw size={13} /> Refresh
        </button>
        <button className="btn btn--sm btn--primary" onClick={() => setShowForm((v) => !v)}>
          + Akun SFTP
        </button>
      </div>

      {showForm && (
        <div className="docker-recreate__row">
          <input placeholder="username" value={username} onChange={(e) => setUsername(e.target.value)} />
          <input type="password" placeholder="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          <button className="btn btn--sm btn--primary" disabled={busy || !username.trim() || !password.trim()} onClick={() => void handleCreate()}>
            Buat
          </button>
        </div>
      )}

      <table className="files-panel__table">
        <thead>
          <tr>
            <th>Username</th>
            <th>Home login</th>
            <th>Status</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list.accounts.map((a) => (
            <tr key={a.username}>
              <td>{a.username}</td>
              <td>{a.homeDir}</td>
              <td>
                <span className={`docker-badge ${a.enabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
                  {a.enabled ? 'Aktif' : 'Terkunci'}
                </span>
              </td>
              <td className="files-panel__row-actions">
                <button disabled={busy} onClick={() => void handleDelete(a.username)}>
                  <Trash2 size={14} />
                </button>
              </td>
            </tr>
          ))}
          {list.accounts.length === 0 && (
            <tr>
              <td colSpan={4} className="files-panel__empty">
                Belum ada akun SFTP untuk domain ini.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
