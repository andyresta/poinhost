import { useEffect, useRef, useState } from 'react';
import {
  GetWebsitePHPStatus,
  SetWebsitePHP,
  DisableWebsitePHP,
  GetWebsiteSSLStatus,
  IssueWebsiteSSL,
  EnableWebsiteSSL,
  DisableWebsiteSSL,
  RenewWebsiteSSL,
  EnableWebsiteSSLAutoRenew,
  StreamWebsiteInstall,
  StopDockerStream,
} from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { DomainFilesTab } from './DomainFilesTab';
import { DomainLogsTab } from './DomainLogsTab';
import { DomainProxyTab } from './DomainProxyTab';
import { DomainDNSTab } from './DomainDNSTab';
import { DomainSFTPTab } from './DomainSFTPTab';
import { DomainCronTab } from './DomainCronTab';
import { DatabaseManagerPanel } from './DatabaseManagerPanel';
import { X } from 'lucide-react';

const SSL_EMAIL_KEY = 'poinhost.website.sslEmail';

const TABS = [
  { key: 'php', label: 'PHP' },
  { key: 'ssl', label: 'SSL' },
  { key: 'files', label: 'Files' },
  { key: 'logs', label: 'Logs' },
  { key: 'proxy', label: 'Proxy' },
  { key: 'database', label: 'Database' },
  { key: 'cron', label: 'Cron' },
  { key: 'dns', label: 'DNS' },
  { key: 'sftp', label: 'SFTP' },
] as const;
type TabKey = (typeof TABS)[number]['key'];

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

function PHPTab({ serverId, domain, onChanged }: { serverId: string; domain: string; onChanged: () => void }) {
  const [status, setStatus] = useState<website.PHPStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [installing, setInstalling] = useState<string | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const streamIdRef = useRef<string | null>(null);

  async function load() {
    try {
      setStatus(await GetWebsitePHPStatus(serverId, domain));
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
  }, [serverId, domain]);

  async function activate(version: string) {
    setBusy(version);
    setError(null);
    try {
      await SetWebsitePHP(new website.PHPSetDomainRequest({ serverId, domain, version }));
      await load();
      onChanged();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function deactivate() {
    setBusy('__disable__');
    setError(null);
    try {
      await DisableWebsitePHP(serverId, domain);
      await load();
      onChanged();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function installVersion(version: string) {
    setInstalling(version);
    setLines([]);
    setError(null);
    const streamId = await StreamWebsiteInstall(serverId, 'php', version);
    streamIdRef.current = streamId;
    const unsub = EventsOn(`website:install:${streamId}`, (evt: StreamLineEvent) => {
      if (evt.type === 'line' && evt.line) setLines((l) => [...l, evt.line as string]);
      else if (evt.type === 'error') {
        setError(evt.message ?? 'Instalasi gagal');
        setInstalling(null);
        streamIdRef.current = null;
        unsub();
      } else if (evt.type === 'end') {
        setInstalling(null);
        streamIdRef.current = null;
        unsub();
        void load();
      }
    });
  }

  async function setupRepo() {
    if (!status) return;
    setInstalling('__repo__');
    setLines([]);
    setError(null);
    const streamId = await StreamWebsiteInstall(serverId, 'php-repo', '');
    streamIdRef.current = streamId;
    const unsub = EventsOn(`website:install:${streamId}`, (evt: StreamLineEvent) => {
      if (evt.type === 'line' && evt.line) setLines((l) => [...l, evt.line as string]);
      else if (evt.type === 'error') {
        setError(evt.message ?? 'Instalasi gagal');
        setInstalling(null);
        streamIdRef.current = null;
        unsub();
      } else if (evt.type === 'end') {
        setInstalling(null);
        streamIdRef.current = null;
        unsub();
        void load();
      }
    });
  }

  if (!status) return <p className="workspace__placeholder">Memuat…</p>;

  return (
    <div>
      {error && <p className="overview__error">{error}</p>}

      {status.versions.length > 0 && (
        <table className="files-panel__table">
          <thead>
            <tr>
              <th>Versi</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {status.versions.map((v) => {
              const active = status.domainVersion === v.version && status.domainEnabled;
              return (
                <tr key={v.version}>
                  <td>PHP {v.version}</td>
                  <td>{v.active ? 'Berjalan' : 'Terpasang'}</td>
                  <td className="files-panel__row-actions">
                    {active ? (
                      <button className="btn btn--sm" disabled={busy === '__disable__'} onClick={() => void deactivate()}>
                        Nonaktifkan
                      </button>
                    ) : (
                      <button className="btn btn--sm btn--primary" disabled={busy === v.version} onClick={() => void activate(v.version)}>
                        Pakai untuk domain ini
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {!status.repoConfigured && status.canInstall && (
        <div className="docker-engine">
          <p>Repo paket PHP (multi-versi) belum disiapkan di server ini.</p>
          <button className="btn btn--primary" disabled={installing === '__repo__'} onClick={() => void setupRepo()}>
            {installing === '__repo__' ? 'Menyiapkan…' : 'Siapkan repo PHP'}
          </button>
        </div>
      )}

      {status.repoConfigured && status.available.length > 0 && (
        <div className="docker-engine">
          <p>Versi PHP lain yang bisa dipasang:</p>
          <div className="files-panel__toolbar">
            {status.available.map((v) => (
              <button key={v} className="btn btn--sm" disabled={!!installing} onClick={() => void installVersion(v)}>
                {installing === v ? 'Menginstal…' : `Install PHP ${v}`}
              </button>
            ))}
          </div>
        </div>
      )}

      {lines.length > 0 && <pre className="docker-engine__log">{lines.join('\n')}</pre>}
    </div>
  );
}

function SSLTab({ serverId, domain, onChanged }: { serverId: string; domain: string; onChanged: () => void }) {
  const [status, setStatus] = useState<website.SSLStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [email, setEmail] = useState(() => localStorage.getItem(SSL_EMAIL_KEY) ?? '');
  const [installing, setInstalling] = useState(false);
  const [lines, setLines] = useState<string[]>([]);
  const streamIdRef = useRef<string | null>(null);

  async function load() {
    try {
      setStatus(await GetWebsiteSSLStatus(serverId, domain));
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
  }, [serverId, domain]);

  async function issueAndEnable() {
    if (!email.trim()) {
      setError('Email wajib diisi untuk menerbitkan sertifikat SSL');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      localStorage.setItem(SSL_EMAIL_KEY, email.trim());
      await IssueWebsiteSSL(new website.SSLIssueRequest({ serverId, domain, email: email.trim() }));
      await EnableWebsiteSSL(serverId, domain);
      await load();
      onChanged();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function enable() {
    setBusy(true);
    setError(null);
    try {
      await EnableWebsiteSSL(serverId, domain);
      await load();
      onChanged();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function disable() {
    setBusy(true);
    setError(null);
    try {
      await DisableWebsiteSSL(serverId, domain);
      await load();
      onChanged();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function renew() {
    setBusy(true);
    setError(null);
    try {
      await RenewWebsiteSSL(serverId, domain);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function enableAutoRenew() {
    setBusy(true);
    setError(null);
    try {
      await EnableWebsiteSSLAutoRenew(serverId, domain);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function installCertbot() {
    setInstalling(true);
    setLines([]);
    setError(null);
    const streamId = await StreamWebsiteInstall(serverId, 'certbot', '');
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

  if (!status) return <p className="workspace__placeholder">Memuat…</p>;

  if (!status.isParent) {
    return <p className="workspace__placeholder">{status.message}</p>;
  }

  if (!status.certbotInstalled) {
    return (
      <div className="docker-engine">
        <p>Certbot belum terpasang di server ini{status.distroName ? ` (distro: ${status.distroName})` : ''}.</p>
        {!status.canInstall && <p className="overview__error">Distro server ini belum didukung instalasi otomatis.</p>}
        {status.canInstall && (
          <button className="btn btn--primary" disabled={installing} onClick={() => void installCertbot()}>
            {installing ? 'Menginstal…' : 'Install Certbot'}
          </button>
        )}
        {error && <p className="overview__error">{error}</p>}
        {lines.length > 0 && <pre className="docker-engine__log">{lines.join('\n')}</pre>}
      </div>
    );
  }

  return (
    <div>
      {error && <p className="overview__error">{error}</p>}
      <p>
        Status:{' '}
        <span className={`docker-badge ${status.sslEnabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
          {status.sslEnabled ? 'SSL aktif' : status.certificate.exists ? 'Sertifikat siap, belum aktif' : 'SSL belum aktif'}
        </span>
      </p>
      {status.sans && status.sans.length > 0 && (
        <p className="chmod-path">Mencakup: {status.sans.join(', ')}</p>
      )}
      {status.certificate.notAfter && <p className="chmod-path">Berlaku sampai: {status.certificate.notAfter}</p>}

      {status.certificate.exists && (
        <p>
          Auto-renew:{' '}
          <span className={`docker-badge ${status.autoRenewEnabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
            {status.autoRenewEnabled ? 'Aktif (cek harian jam 03:12, reload Nginx otomatis)' : 'Belum terpasang'}
          </span>
          {!status.autoRenewEnabled && (
            <button className="btn btn--sm" style={{ marginLeft: 8 }} disabled={busy} onClick={() => void enableAutoRenew()}>
              Pasang auto-renew
            </button>
          )}
        </p>
      )}

      {!status.certificate.exists && (
        <>
          <label className="form-field">
            <span>Email (untuk Let's Encrypt)</span>
            <input type="email" placeholder="admin@contoh.com" value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <p className="chmod-path">
            Pastikan DNS domain ini sudah diarahkan ke IP server sebelum menerbitkan sertifikat.
          </p>
          <button className="btn btn--primary" disabled={busy} onClick={() => void issueAndEnable()}>
            {busy ? 'Memproses…' : "Terbitkan & Aktifkan SSL"}
          </button>
        </>
      )}

      {status.certificate.exists && !status.sslEnabled && (
        <button className="btn btn--primary" disabled={busy} onClick={() => void enable()}>
          Aktifkan SSL
        </button>
      )}
      {status.certificate.exists && status.sslEnabled && (
        <div className="files-panel__toolbar">
          <button className="btn btn--sm" disabled={busy} onClick={() => void renew()}>
            Perbarui sertifikat
          </button>
          <button className="btn btn--sm btn--danger" disabled={busy} onClick={() => void disable()}>
            Nonaktifkan SSL
          </button>
        </div>
      )}
    </div>
  );
}

// Panel detail satu domain — sub-nav lengkap 9 tab (PHP, SSL, Files, Logs,
// Proxy, DNS, SFTP, Cron, Database), semuanya berfungsi penuh. Ini
// melengkapi seluruh menu Website yang ada di homepoin (lihat
// ARCHITECTURE.md §Website untuk detail tiap tab & perbedaan arsitekturnya).
export function DomainDetailModal({
  serverId,
  domain,
  onClose,
  onChanged,
}: {
  serverId: string;
  domain: string;
  onClose: () => void;
  onChanged: () => void;
}) {
  const [tab, setTab] = useState<TabKey>('php');

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--editor">
        <div className="modal-card__header">
          <h2>{domain}</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body modal-card__body--editor">
          <div className="subnav">
            {TABS.map((t) => (
              <button key={t.key} className={tab === t.key ? 'active' : ''} onClick={() => setTab(t.key)}>
                {t.label}
              </button>
            ))}
          </div>

          {tab === 'php' && <PHPTab serverId={serverId} domain={domain} onChanged={onChanged} />}
          {tab === 'ssl' && <SSLTab serverId={serverId} domain={domain} onChanged={onChanged} />}
          {tab === 'files' && <DomainFilesTab serverId={serverId} domain={domain} />}
          {tab === 'logs' && <DomainLogsTab serverId={serverId} domain={domain} />}
          {tab === 'proxy' && <DomainProxyTab serverId={serverId} domain={domain} />}
          {tab === 'dns' && <DomainDNSTab serverId={serverId} domain={domain} />}
          {tab === 'sftp' && <DomainSFTPTab serverId={serverId} domain={domain} />}
          {tab === 'cron' && <DomainCronTab serverId={serverId} domain={domain} />}
          {tab === 'database' && <DatabaseManagerPanel serverId={serverId} domain={domain} />}
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Tutup
          </button>
        </div>
      </div>
    </div>
  );
}
