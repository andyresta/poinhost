import { useCallback, useEffect, useState } from 'react';
import {
  CircleCheck,
  CircleSlash,
  Copy,
  Download,
  Eye,
  EyeOff,
  FileText,
  Globe,
  KeyRound,
  Power,
  RotateCw,
  ServerCog,
  Smartphone,
  Trash2,
  TriangleAlert,
} from 'lucide-react';
import {
  DetectAgent,
  InstallAgent,
  UpdateAgent,
  UninstallAgent,
  EnableAgentSelfManagement,
  AgentLifecycle,
  AgentLogs,
  MobileControlToken,
  RotateMobileControlToken,
  MobileControlPublishStatus,
  MobileControlPublish,
  MobileControlUnpublish,
} from '../../../wailsjs/go/main/App';
import { agentctl } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { useConfirm } from '../../components/ConfirmDialog';
import { useT } from '../../i18n';
import type { MessageKey } from '../../i18n/messages';

// Dikirim lewat event `agent:progress:<serverId>`, bukan sebagai return value
// binding — Wails hanya membuat model untuk tipe yang dipakai method yang
// di-bind, jadi bentuknya ditulis di sini. Harus selaras dengan
// agentctl.Progress di Go.
type AgentProgress = { serverId: string; phase: string; percent: number };

// Modul Mobile Control.
//
// Memasang poinhost-agent sebagai service systemd di server, lalu menyediakan
// token yang dipakai aplikasi di HP untuk menyambung. Agent hanya mengurus
// MESINNYA SENDIRI: tidak ada penautan server lain, jadi tiap server yang
// ingin dilihat dari HP memasang agent-nya sendiri.
//
// Agent selalu mendengarkan di 127.0.0.1 saja — yang menjangkaunya dari luar
// adalah reverse proxy nginx + certbot lewat modul Website, bukan port agent
// yang dibuka ke publik.
export function MobileControlPanel({ serverId }: { serverId: string }) {
  const t = useT();
  const confirm = useConfirm();

  const [status, setStatus] = useState<agentctl.Status | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [port, setPort] = useState(7898);
  const [logs, setLogs] = useState<string | null>(null);
  const [progress, setProgress] = useState<AgentProgress | null>(null);

  // Token tidak pernah disimpan di sisi desktop — ia dibaca dari config agent
  // saat user memintanya, dan dilupakan lagi begitu pindah server.
  const [token, setToken] = useState<agentctl.AccessToken | null>(null);

  // Jalur dari HP ke agent: vhost nginx yang mem-proxy ke port loopback.
  const [publish, setPublish] = useState<agentctl.PublishInfo | null>(null);
  const [publishError, setPublishError] = useState<string | null>(null);
  const [pubDomain, setPubDomain] = useState('');
  const [pubEmail, setPubEmail] = useState('');

  const load = useCallback(async () => {
    setError(null);
    try {
      setStatus(await DetectAgent(serverId));
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [serverId]);

  // Keadaan terbit dibaca dari SERVER (vhost nginx yang ada sekarang), bukan
  // disimpan di sisi desktop — kalau seseorang mengubah vhost-nya langsung,
  // panel ini harus ikut benar.
  const loadPublish = useCallback(async () => {
    setPublishError(null);
    try {
      const info = await MobileControlPublishStatus(serverId);
      setPublish(info);
      // Domain yang sudah terbit diisikan ke form, supaya "terbitkan ulang"
      // tidak menuntut user mengetik ulang nama yang sudah dipakai.
      if (info.domain) setPubDomain(info.domain);
    } catch (e) {
      // Kegagalan DITAMPILKAN, bukan disembunyikan: bagian yang hilang saat
      // gagal membuat fiturnya seolah tidak ada.
      setPublish(null);
      setPublishError(String(e));
    }
  }, [serverId]);

  useEffect(() => {
    setLoading(true);
    setLogs(null);
    setToken(null);
    setPublish(null);
    setPublishError(null);
    setPubDomain('');
    setPubEmail('');
    void load();
  }, [serverId, load]);

  useEffect(() => {
    if (status?.state === 'running') void loadPublish();
  }, [status?.state, loadPublish]);

  // Kemajuan pemasangan/update datang per server, supaya tab lain yang sedang
  // menjalankan operasinya sendiri tidak saling menimpa.
  useEffect(() => EventsOn(`agent:progress:${serverId}`, (raw: unknown) => {
    setProgress(raw as AgentProgress);
  }), [serverId]);

  // Setiap aksi menghasilkan Status baru dari backend, bukan menebak keadaan
  // dari sisi UI — perbedaan antara "perintah terkirim" dan "agent benar-benar
  // sehat" persis yang bikin layar seperti ini menyesatkan.
  async function act<T>(key: string, fn: () => Promise<T>, after?: (r: T) => void) {
    setBusy(key);
    setError(null);
    setProgress(null);
    try {
      const res = await fn();
      if (after) after(res);
      else setStatus(res as unknown as agentctl.Status);
    } catch (e) {
      setError(String(e));
      void load();
    } finally {
      setBusy(null);
      setProgress(null);
    }
  }

  async function handleUninstall() {
    const ok = await confirm({
      title: t('mobile.uninstall'),
      message: t('mobile.uninstallConfirm'),
      detail: t('mobile.uninstallDetail'),
      confirmLabel: t('mobile.uninstall'),
      danger: true,
    });
    if (!ok) return;
    void act('uninstall', () => UninstallAgent(serverId));
  }

  // Mengganti token memutus setiap HP yang masih memakai yang lama, jadi
  // konsekuensinya ditaruh di dialog — bukan sebagai paragraf di panel.
  async function handleRotate() {
    if (status?.apiConfigured) {
      const ok = await confirm({
        title: t('mobile.replaceToken'),
        message: t('mobile.replaceConfirm'),
        detail: t('mobile.replaceDetail'),
        confirmLabel: t('mobile.replaceToken'),
        danger: true,
      });
      if (!ok) return;
    }
    void act('rotate', () => RotateMobileControlToken(serverId), (tok) => {
      setToken(tok);
      void load();
    });
  }

  async function handleUnpublish() {
    const ok = await confirm({
      title: t('mobile.unpublish'),
      message: t('mobile.unpublishConfirm', { domain: publish?.domain ?? '' }),
      detail: t('mobile.unpublishDetail'),
      confirmLabel: t('mobile.unpublish'),
      danger: true,
    });
    if (!ok) return;
    void act('unpublish', () => MobileControlUnpublish(serverId), (info) => setPublish(info));
  }

  if (loading) return <p className="workspace__placeholder">{t('common.loading')}</p>;
  if (!status) {
    return (
      <div className="files-panel">
        <p className="fw-notice fw-notice--warn">
          <TriangleAlert size={13} /> {error ?? t('common.error')}
        </p>
        <button className="btn" onClick={() => void load()}>
          <RotateCw size={13} /> {t('common.retry')}
        </button>
      </div>
    );
  }

  const running = status.state === 'running';
  const installed = status.installed;
  const anyBusy = busy !== null;

  const wantDomain = pubDomain.trim().toLowerCase();
  const domainTaken = (publish?.candidates ?? []).some((c) => c.domain === wantDomain && c.taken);
  const isReissue = publish?.domain !== '' && publish?.domain === wantDomain;

  return (
    <div className="files-panel">
      {error && (
        <p className="fw-notice fw-notice--warn">
          <TriangleAlert size={13} /> {error}
        </p>
      )}

      {/* ---- status agent ---- */}
      <div className="fw-status">
        <span className={`docker-badge ${running ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
          {running ? <CircleCheck size={12} /> : <CircleSlash size={12} />} {t(stateKey(status.state))}
        </span>
        {status.version && (
          <span className="fw-status__item">
            {t('mobile.version')}: <strong>{status.version}</strong>
          </span>
        )}
        {status.listen && (
          <span className="fw-status__item">
            {t('mobile.listen')}: <strong>{status.listen}</strong>
          </span>
        )}
        {running && (status.uptimeSeconds ?? 0) > 0 && (
          <span className="fw-status__item">
            {t('mobile.uptime')}: <strong>{uptime(status.uptimeSeconds ?? 0)}</strong>
          </span>
        )}
        {installed && (
          <span className="fw-status__item">
            {t('mobile.autostart')}: <strong>{status.enabled ? t('common.yes') : t('common.no')}</strong>
          </span>
        )}
        <button className="btn btn--ghost" disabled={anyBusy} onClick={() => void load()}>
          <RotateCw size={13} /> {t('common.refresh')}
        </button>
      </div>

      {/* Progress ditampilkan untuk operasi yang benar-benar panjang saja —
          unggahan binary plus restart service. Aksi pendek cukup spinner. */}
      {progress && (busy === 'install' || busy === 'update' || busy === 'selfmanage') && (
        <div className="agent-progress">
          <div className="agent-progress__bar">
            <div className="agent-progress__fill" style={{ width: `${progress.percent}%` }} />
          </div>
          <span className="agent-progress__label">
            {t(phaseKey(progress.phase))} · {progress.percent}%
          </span>
        </div>
      )}

      {status.problem && (
        <p className="fw-notice fw-notice--warn">
          <TriangleAlert size={13} /> {status.problem}
        </p>
      )}

      {/* Agent terpasang tapi tidak menjawab health check adalah keadaan yang
          paling membingungkan kalau tidak dijelaskan: service-nya "aktif" di
          mata systemd, tapi prosesnya gagal melayani. */}
      {installed && status.active && !status.healthy && (
        <p className="fw-notice fw-notice--warn">
          <TriangleAlert size={13} /> {t('mobile.unhealthy')}
        </p>
      )}

      {/* ---- belum terpasang: form pasang ---- */}
      {!installed && (
        <section className="agent-section">
          <h3>{t('mobile.install')}</h3>
          <p className="agent-hint">{t('mobile.installIntro')}</p>

          <label className="agent-field">
            <span>{t('mobile.port')}</span>
            <input
              type="number"
              value={port}
              min={1}
              max={65535}
              onChange={(e) => setPort(Number(e.target.value))}
            />
          </label>
          <p className="agent-hint">{t('mobile.portHint')}</p>

          {/* Pengelolaan-diri TIDAK lagi opsional: satu agent hanya mengurus
              mesinnya sendiri, jadi agent tanpa itu tidak melaporkan apa pun.
              Menawarkannya sebagai pilihan cuma mengundang pemasangan yang
              tidak berguna. */}
          <button
            className="btn btn--primary"
            disabled={anyBusy || !status.archSupported || !status.hasSystemd || !status.canElevate || !status.bundleAvailable}
            onClick={() => void act('install', () => InstallAgent({ serverId, port, manageSelf: true } as agentctl.InstallRequest))}
          >
            {busy === 'install' ? <span className="spinner" /> : <Download size={13} />} {t('mobile.install')}
          </button>
        </section>
      )}

      {/* ---- akses aplikasi ---- */}
      {installed && (
        <section className="agent-section">
          <h3>
            <Smartphone size={14} /> {t('mobile.access')}
          </h3>

          {status.apiConfigured ? (
            <p className="fw-notice">
              <CircleCheck size={13} /> {t('mobile.tokenSet')}
            </p>
          ) : (
            <p className="fw-notice fw-notice--warn">
              <TriangleAlert size={13} /> {t('mobile.tokenMissing')}
            </p>
          )}

          {token && (
            <div className="agent-pairing">
              <code className="agent-pairing__code">{token.token}</code>
              <button
                className="btn btn--ghost"
                onClick={() => void navigator.clipboard?.writeText(token.token)}
                title={t('common.copy')}
              >
                <Copy size={13} />
              </button>
              <button className="btn btn--ghost" onClick={() => setToken(null)}>
                <EyeOff size={13} /> {t('mobile.hideToken')}
              </button>
              <span className="agent-hint">
                {t('mobile.endpoint')}: <code>{publish?.url || `127.0.0.1:${token.port}`}</code>
              </span>
            </div>
          )}

          <div className="agent-actions">
            {status.apiConfigured && !token && (
              <button
                className="btn"
                disabled={anyBusy}
                onClick={() => void act('token', () => MobileControlToken(serverId), (tok) => setToken(tok))}
              >
                {busy === 'token' ? <span className="spinner" /> : <Eye size={13} />} {t('mobile.showToken')}
              </button>
            )}
            <button
              className={status.apiConfigured ? 'btn' : 'btn btn--primary'}
              disabled={anyBusy}
              onClick={() => void handleRotate()}
            >
              {busy === 'rotate' ? <span className="spinner" /> : <KeyRound size={13} />}{' '}
              {status.apiConfigured ? t('mobile.replaceToken') : t('mobile.newToken')}
            </button>
          </div>

          <p className="agent-hint">{t('mobile.tokenHint')}</p>
        </section>
      )}

      {/* ---- jalur dari HP: vhost nginx + certbot ---- */}
      {running && (
        <section className="agent-section">
          <h3>
            <Globe size={14} /> {t('mobile.publishSection')}
          </h3>

          {publishError && (
            <>
              <p className="fw-notice fw-notice--warn">
                <TriangleAlert size={13} /> {publishError}
              </p>
              <button className="btn" disabled={anyBusy} onClick={() => void loadPublish()}>
                <RotateCw size={13} /> {t('common.retry')}
              </button>
            </>
          )}

          {publish && (
            <>
              {publish.domain ? (
                <div className="agent-pairing">
                  <code className="agent-pairing__code">{publish.url}</code>
                  <button
                    className="btn btn--ghost"
                    onClick={() => void navigator.clipboard?.writeText(publish.url ?? '')}
                    title={t('common.copy')}
                  >
                    <Copy size={13} />
                  </button>
                  {!publish.sslEnabled && (
                    <span className="docker-badge docker-badge--stopped">{t('mobile.noTLS')}</span>
                  )}
                </div>
              ) : (
                <p className="fw-notice">
                  <CircleSlash size={13} /> {t('mobile.notPublished')}
                </p>
              )}

              {/* Problem datang dari backend: sertifikat gagal terbit, atau
                  terbit tapi belum terpasang. Proxy-nya sendiri sudah jalan,
                  jadi ini peringatan, bukan kegagalan seluruh operasi. */}
              {publish.problem && (
                <p className="fw-notice fw-notice--warn">
                  <TriangleAlert size={13} /> {publish.problem}
                </p>
              )}
              {!publish.nginxInstalled && (
                <p className="fw-notice fw-notice--warn">
                  <TriangleAlert size={13} /> {t('mobile.needNginx')}
                </p>
              )}
              {publish.nginxInstalled && !publish.certbotInstalled && (
                <p className="fw-notice">
                  <TriangleAlert size={13} /> {t('mobile.needCertbot')}
                </p>
              )}

              {/* Daftar domain server ini ditawarkan lewat datalist, bukan
                  <select>: domain yang belum ada di server pun harus bisa
                  diketik, karena subdomain untuk agent biasanya memang baru. */}
              <label className="agent-field">
                <span>{t('mobile.domain')}</span>
                <input
                  type="text"
                  list={`mobile-domains-${serverId}`}
                  value={pubDomain}
                  placeholder={t('mobile.domainPlaceholder')}
                  onChange={(e) => setPubDomain(e.target.value)}
                />
              </label>
              <datalist id={`mobile-domains-${serverId}`}>
                {publish.candidates
                  .filter((c) => !c.taken)
                  .map((c) => (
                    <option key={c.domain} value={c.domain} />
                  ))}
              </datalist>
              {domainTaken && (
                <p className="fw-notice fw-notice--warn">
                  <TriangleAlert size={13} /> {t('mobile.domainTaken')}
                </p>
              )}

              <label className="agent-field">
                <span>{t('mobile.email')}</span>
                <input
                  type="email"
                  value={pubEmail}
                  placeholder="you@example.com"
                  onChange={(e) => setPubEmail(e.target.value)}
                />
              </label>
              <p className="agent-hint">{t('mobile.emailHint')}</p>

              <div className="agent-actions">
                <button
                  className="btn btn--primary"
                  disabled={anyBusy || !wantDomain || domainTaken || !publish.nginxInstalled}
                  onClick={() =>
                    void act(
                      'publish',
                      () =>
                        MobileControlPublish({
                          serverId,
                          domain: wantDomain,
                          email: pubEmail.trim(),
                        } as agentctl.PublishRequest),
                      (info) => setPublish(info),
                    )
                  }
                >
                  {busy === 'publish' ? <span className="spinner" /> : <Globe size={13} />}{' '}
                  {isReissue ? t('mobile.reissue') : t('mobile.publish')}
                </button>

                {publish.domain && (
                  <button className="btn btn--danger" disabled={anyBusy} onClick={() => void handleUnpublish()}>
                    <Trash2 size={13} /> {t('mobile.unpublish')}
                  </button>
                )}
              </div>

              <p className="agent-hint">{t('mobile.publishHint')}</p>
            </>
          )}
        </section>
      )}

      {/* ---- terpasang: siklus hidup ---- */}
      {installed && (
        <section className="agent-section">
          <div className="agent-actions">
            {running ? (
              <>
                <button className="btn" disabled={anyBusy} onClick={() => void act('restart', () => AgentLifecycle(serverId, 'restart'))}>
                  {busy === 'restart' ? <span className="spinner" /> : <RotateCw size={13} />} {t('mobile.restart')}
                </button>
                <button className="btn" disabled={anyBusy} onClick={() => void act('stop', () => AgentLifecycle(serverId, 'stop'))}>
                  <Power size={13} /> {t('mobile.stop')}
                </button>
              </>
            ) : (
              <button className="btn btn--primary" disabled={anyBusy} onClick={() => void act('start', () => AgentLifecycle(serverId, 'start'))}>
                {busy === 'start' ? <span className="spinner" /> : <Power size={13} />} {t('mobile.start')}
              </button>
            )}

            <button
              className="btn"
              disabled={anyBusy}
              onClick={() => void act('logs', () => AgentLogs(serverId, 200), (out) => setLogs(out))}
            >
              {busy === 'logs' ? <span className="spinner" /> : <FileText size={13} />} {t('mobile.logs')}
            </button>

            {/* Hanya muncul kalau versi yang dibundel terbukti lebih baru dari
                yang terpasang — bukan sekadar berbeda. */}
            {status.updateAvailable && (
              <button className="btn btn--primary" disabled={anyBusy} onClick={() => void act('update', () => UpdateAgent(serverId))}>
                {busy === 'update' ? <span className="spinner" /> : <Download size={13} />}{' '}
                {t('mobile.updateTo', { version: status.bundledVersion ?? '' })}
              </button>
            )}

            <button className="btn btn--danger" disabled={anyBusy} onClick={() => void handleUninstall()}>
              <Trash2 size={13} /> {t('mobile.uninstall')}
            </button>
          </div>

          {/* Aksinya SELALU tersedia selama agent terpasang, bukan hanya saat
              pengelolaan-diri mati. Menyalakannya dan benar-benar bisa
              menyambung adalah dua hal berbeda — mis. known_hosts agent belum
              terisi — dan tanpa jalan memasang ulang kuncinya, satu-satunya
              pemulihan adalah copot-pasang yang juga membuang token akses.
              Operasinya idempoten: kunci lama diganti, bukan ditumpuk. */}
          <div className="agent-selfmanage">
            {!status.selfManaged && (
              <p className="fw-notice fw-notice--warn">
                <TriangleAlert size={13} /> {t('mobile.selfOff')}
              </p>
            )}
            <button
              className={status.selfManaged ? 'btn' : 'btn btn--primary'}
              disabled={anyBusy}
              title={status.selfManaged ? t('mobile.selfRepairTitle') : undefined}
              onClick={() => void act('selfmanage', () => EnableAgentSelfManagement(serverId))}
            >
              {busy === 'selfmanage' ? <span className="spinner" /> : <ServerCog size={13} />}{' '}
              {status.selfManaged ? t('mobile.selfRepair') : t('mobile.enableSelf')}
            </button>
          </div>

          {logs !== null && (
            <pre className="agent-logs" onDoubleClick={() => setLogs(null)}>
              {logs}
            </pre>
          )}
        </section>
      )}
    </div>
  );
}

function phaseKey(phase: string): MessageKey {
  switch (phase) {
    case 'upload':
      return 'mobile.phaseUpload';
    case 'verify':
      return 'mobile.phaseVerify';
    case 'install':
      return 'mobile.phaseInstall';
    case 'health':
      return 'mobile.phaseHealth';
    case 'done':
      return 'mobile.phaseDone';
    default:
      return 'mobile.phasePrepare';
  }
}

function stateKey(state: string): MessageKey {
  switch (state) {
    case 'running':
      return 'mobile.stateRunning';
    case 'stopped':
      return 'mobile.stateStopped';
    default:
      return 'mobile.stateNotInstalled';
  }
}

function uptime(sec: number) {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
