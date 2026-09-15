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
import { ArrowLeft, CornerDownRight, Lock, Plus, Pause, Play, Trash2 } from 'lucide-react';
import { DomainFeatureGrid, featureLabel, type FeatureKey } from './DomainFeatureGrid';
import { useT } from '../../i18n';

const SSL_EMAIL_KEY = 'poinhost.website.sslEmail';

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
                        {busy === '__disable__' && <span className="spinner" />} Nonaktifkan
                      </button>
                    ) : (
                      <button className="btn btn--sm btn--primary" disabled={busy === v.version} onClick={() => void activate(v.version)}>
                        {busy === v.version && <span className="spinner" />} Pakai untuk domain ini
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
            {installing === '__repo__' && <span className="spinner" />} {installing === '__repo__' ? 'Menyiapkan…' : 'Siapkan repo PHP'}
          </button>
        </div>
      )}

      {status.repoConfigured && status.available.length > 0 && (
        <div className="docker-engine">
          <p>Versi PHP lain yang bisa dipasang:</p>
          <div className="files-panel__toolbar">
            {status.available.map((v) => (
              <button key={v} className="btn btn--sm" disabled={!!installing} onClick={() => void installVersion(v)}>
                {installing === v && <span className="spinner" />} {installing === v ? 'Menginstal…' : `Install PHP ${v}`}
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
            {installing && <span className="spinner" />} {installing ? 'Menginstal…' : 'Install Certbot'}
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
      {status.sans && status.sans.length > 0 && <p className="chmod-path">Mencakup: {status.sans.join(', ')}</p>}
      {status.certificate.notAfter && <p className="chmod-path">Berlaku sampai: {status.certificate.notAfter}</p>}

      {status.certificate.exists && (
        <p>
          Auto-renew:{' '}
          <span className={`docker-badge ${status.autoRenewEnabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
            {status.autoRenewEnabled ? 'Aktif (cek harian jam 03:12, reload Nginx otomatis)' : 'Belum terpasang'}
          </span>
          {!status.autoRenewEnabled && (
            <button className="btn btn--sm" style={{ marginLeft: 8 }} disabled={busy} onClick={() => void enableAutoRenew()}>
              {busy && <span className="spinner" />} Pasang auto-renew
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
          <p className="chmod-path">Pastikan DNS domain ini sudah diarahkan ke IP server sebelum menerbitkan sertifikat.</p>
          <button className="btn btn--primary" disabled={busy} onClick={() => void issueAndEnable()}>
            {busy && <span className="spinner" />} {busy ? 'Memproses…' : "Terbitkan & Aktifkan SSL"}
          </button>
        </>
      )}

      {status.certificate.exists && !status.sslEnabled && (
        <button className="btn btn--primary" disabled={busy} onClick={() => void enable()}>
          {busy && <span className="spinner" />} Aktifkan SSL
        </button>
      )}
      {status.certificate.exists && status.sslEnabled && (
        <div className="files-panel__toolbar">
          <button className="btn btn--sm" disabled={busy} onClick={() => void renew()}>
            {busy && <span className="spinner" />} Perbarui sertifikat
          </button>
          <button className="btn btn--sm btn--danger" disabled={busy} onClick={() => void disable()}>
            {busy && <span className="spinner" />} Nonaktifkan SSL
          </button>
        </div>
      )}
    </div>
  );
}


// Panel detail satu domain — accordion collapse untuk 9 fitur (PHP, SSL,
// Files, Logs, Proxy, DNS, SFTP, Cron, Database), dirender INLINE menggantikan
// daftar domain di WebsitePanel (bukan modal overlay) — permintaan user
// eksplisit: modal per-fitur "kurang baik secara UI" untuk sebanyak ini.
// Melengkapi seluruh menu Website homepoin (lihat ARCHITECTURE.md §Website).
export function DomainDetailPanel({
  serverId,
  domain,
  onBack,
  onChanged,
  onToggleEnabled,
  onAddSubdomain,
  onDelete,
  busy,
  initialFeature,
}: {
  serverId: string;
  domain: website.DomainInfo;
  onBack: () => void;
  onChanged: () => void;
  onToggleEnabled: () => void;
  onAddSubdomain: () => void;
  onDelete: () => void;
  busy: boolean;
  initialFeature?: FeatureKey | null;
}) {
  const name = domain.domain;
  const t = useT();

  // Dashboard grid ikon (model Plesk). Panel ini biasanya dibuka SUDAH pada
  // satu fitur (initialFeature) karena gridnya dipilih lebih dulu inline di
  // daftar domain — "Menu" balik ke grid di sini. Reset setiap ganti
  // domain/fitur supaya tidak nyangkut di halaman fitur domain SEBELUMNYA.
  const [activeFeature, setActiveFeature] = useState<FeatureKey | null>(initialFeature ?? null);
  useEffect(() => {
    setActiveFeature(initialFeature ?? null);
  }, [name, initialFeature]);

  function renderFeature(key: FeatureKey) {
    switch (key) {
      case 'php':
        return <PHPTab serverId={serverId} domain={name} onChanged={onChanged} />;
      case 'ssl':
        return <SSLTab serverId={serverId} domain={name} onChanged={onChanged} />;
      case 'files':
        return <DomainFilesTab serverId={serverId} domain={name} />;
      case 'logs':
        return <DomainLogsTab serverId={serverId} domain={name} />;
      case 'proxy':
        return <DomainProxyTab serverId={serverId} domain={name} />;
      case 'database':
        return <DatabaseManagerPanel serverId={serverId} domain={name} />;
      case 'cron':
        return <DomainCronTab serverId={serverId} domain={name} />;
      case 'dns':
        return <DomainDNSTab serverId={serverId} domain={name} />;
      case 'sftp':
        return <DomainSFTPTab serverId={serverId} domain={name} />;
    }
  }

  return (
    <div className="domain-detail-panel">
      <div className="domain-detail-panel__header">
        <button className="btn btn--ghost btn--sm" onClick={onBack}>
          <ArrowLeft size={14} /> {t('common.back')}
        </button>
        <h2 className="domain-detail-panel__title">
          {domain.isSubdomain && <CornerDownRight size={14} />}
          {name}
        </h2>
        <span className={`docker-badge ${domain.enabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
          {domain.enabled ? t('common.active') : t('common.inactive')}
        </span>
        {domain.phpEnabled && <span className="docker-badge">PHP {domain.phpVersion}</span>}
        {domain.sslEnabled && (
          <span className="docker-badge docker-badge--running">
            <Lock size={11} /> SSL
          </span>
        )}
        <div className="domain-detail-panel__actions">
          {!domain.isSubdomain && (
            <button title={t('web.addSubdomain')} disabled={busy} onClick={onAddSubdomain}>
              <Plus size={14} />
            </button>
          )}
          <button title={domain.enabled ? t('web.disable') : t('web.enable')} disabled={busy} onClick={onToggleEnabled}>
            {busy ? <span className="spinner" /> : domain.enabled ? <Pause size={14} /> : <Play size={14} />}
          </button>
          <button title={t('common.delete')} disabled={busy} onClick={onDelete}>
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      {activeFeature === null ? (
        <DomainFeatureGrid isSubdomain={domain.isSubdomain} onSelect={setActiveFeature} />
      ) : (
        <div>
          <div className="domain-feature-page__header">
            <button className="btn btn--ghost btn--sm" onClick={() => setActiveFeature(null)}>
              <ArrowLeft size={14} /> {t('web.menu')}
            </button>
            <h3>{featureLabel(activeFeature)}</h3>
          </div>
          {renderFeature(activeFeature)}
        </div>
      )}
    </div>
  );
}
