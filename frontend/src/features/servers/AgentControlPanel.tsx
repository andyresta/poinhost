import { useCallback, useEffect, useState } from 'react';
import {
  Bot,
  CircleCheck,
  CircleSlash,
  Copy,
  Download,
  FileText,
  Power,
  Link2,
  RotateCw,
  ServerCog,
  Trash2,
  TriangleAlert,
} from 'lucide-react';
import {
  DetectAgent,
  InstallAgent,
  UpdateAgent,
  UninstallAgent,
  EnableAgentSelfManagement,
  ListAgentLinkedServers,
  LinkServersToAgent,
  UnlinkServerFromAgent,
  AgentLifecycle,
  AgentLogs,
  TestAgentTelegramToken,
  ConfigureAgentTelegram,
  CreateAgentPairingCode,
  RevokeAgentTelegramUser,
} from '../../../wailsjs/go/main/App';
import { agentctl } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { useTabsStore } from '../../store/tabs';
import { useConfirm } from '../../components/ConfirmDialog';
import { useT } from '../../i18n';
import type { MessageKey } from '../../i18n/messages';

// Dikirim lewat event `agent:progress:<serverId>`, bukan sebagai return value
// binding — Wails hanya membuat model untuk tipe yang dipakai method yang
// di-bind, jadi bentuknya ditulis di sini. Harus selaras dengan
// agentctl.Progress di Go.
type AgentProgress = { serverId: string; phase: string; percent: number };

// Modul Agent Control.
//
// Memasang poinhost-agent sebagai service systemd di server, lalu mengatur
// konektor bot Telegram-nya. Agent memakai ULANG modul poinhost yang sama,
// jadi yang dilaporkan bot adalah data yang sama dengan yang dilihat di sini.
//
// Agent selalu mendengarkan di 127.0.0.1 saja — kalau REST API-nya perlu
// dijangkau dari luar, jalurnya reverse proxy nginx + certbot lewat modul
// Website, bukan membuka port agent ke publik.
export function AgentControlPanel({ serverId }: { serverId: string }) {
  const t = useT();
  const confirm = useConfirm();

  const [status, setStatus] = useState<agentctl.Status | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [manageSelf, setManageSelf] = useState(true);
  const [port, setPort] = useState(7898);

  const [token, setToken] = useState('');
  const [tokenTest, setTokenTest] = useState<agentctl.TelegramTestResult | null>(null);
  const [pairing, setPairing] = useState<agentctl.PairingCode | null>(null);
  const [logs, setLogs] = useState<string | null>(null);

  // Server yang dikelola poinhost — inilah yang bisa ditautkan ke agent.
  const allServers = useTabsStore((st) => st.servers);
  const [linked, setLinked] = useState<agentctl.LinkedServer[] | null>(null);
  const [picked, setPicked] = useState<Set<string>>(() => new Set());
  const [progress, setProgress] = useState<AgentProgress | null>(null);
  const [linkError, setLinkError] = useState<string | null>(null);

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

  // Daftar server tertaut dibaca dari AGENT, bukan disimpan di sisi desktop —
  // satu-satunya keadaan yang benar ada di sana.
  const loadLinked = useCallback(async () => {
    setLinkError(null);
    try {
      setLinked(await ListAgentLinkedServers(serverId));
    } catch (e) {
      // Kegagalan di sini DITAMPILKAN, tidak disembunyikan. Menyembunyikan
      // seluruh bagian saat gagal membuat fiturnya seolah tidak ada — user
      // tidak punya cara tahu apakah ia belum dibuat, atau sedang error.
      setLinked(null);
      setLinkError(String(e));
    }
  }, [serverId]);

  useEffect(() => {
    setLoading(true);
    setLogs(null);
    setPairing(null);
    setTokenTest(null);
    setLinked(null);
    setLinkError(null);
    setPicked(new Set());
    void load();
  }, [serverId, load]);

  useEffect(() => {
    if (status?.state === 'running') void loadLinked();
  }, [status?.state, loadLinked]);

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
      title: t('agent.uninstall'),
      message: t('agent.uninstallConfirm'),
      detail: t('agent.uninstallDetail'),
      confirmLabel: t('agent.uninstall'),
      danger: true,
    });
    if (!ok) return;
    void act('uninstall', () => UninstallAgent(serverId));
  }

  async function handleRevoke(userId: number) {
    const ok = await confirm({
      title: t('agent.revoke'),
      message: t('agent.revokeConfirm', { id: String(userId) }),
      confirmLabel: t('agent.revoke'),
      danger: true,
    });
    if (!ok) return;
    void act('revoke', () => RevokeAgentTelegramUser(serverId, userId));
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
            {t('agent.version')}: <strong>{status.version}</strong>
          </span>
        )}
        {status.listen && (
          <span className="fw-status__item">
            {t('agent.listen')}: <strong>{status.listen}</strong>
          </span>
        )}
        {running && (status.uptimeSeconds ?? 0) > 0 && (
          <span className="fw-status__item">
            {t('agent.uptime')}: <strong>{uptime(status.uptimeSeconds ?? 0)}</strong>
          </span>
        )}
        {installed && (
          <span className="fw-status__item">
            {t('agent.autostart')}: <strong>{status.enabled ? t('common.yes') : t('common.no')}</strong>
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
          <TriangleAlert size={13} /> {t('agent.unhealthy')}
        </p>
      )}

      {/* ---- belum terpasang: form pasang ---- */}
      {!installed && (
        <section className="agent-section">
          <h3>{t('agent.install')}</h3>
          <p className="agent-hint">{t('agent.installIntro')}</p>

          <label className="agent-field">
            <span>{t('agent.port')}</span>
            <input
              type="number"
              value={port}
              min={1}
              max={65535}
              onChange={(e) => setPort(Number(e.target.value))}
            />
          </label>
          <p className="agent-hint">{t('agent.portHint')}</p>

          <label className="agent-check">
            <input type="checkbox" checked={manageSelf} onChange={(e) => setManageSelf(e.target.checked)} />
            <span>{t('agent.manageSelf')}</span>
          </label>
          <p className="agent-hint">{t('agent.manageSelfHint')}</p>

          <button
            className="btn btn--primary"
            disabled={anyBusy || !status.archSupported || !status.hasSystemd || !status.canElevate || !status.bundleAvailable}
            onClick={() => void act('install', () => InstallAgent({ serverId, port, manageSelf } as agentctl.InstallRequest))}
          >
            {busy === 'install' ? <span className="spinner" /> : <Download size={13} />} {t('agent.install')}
          </button>
        </section>
      )}

      {/* ---- terpasang: siklus hidup ---- */}
      {installed && (
        <section className="agent-section">
          <div className="agent-actions">
            {running ? (
              <>
                <button className="btn" disabled={anyBusy} onClick={() => void act('restart', () => AgentLifecycle(serverId, 'restart'))}>
                  {busy === 'restart' ? <span className="spinner" /> : <RotateCw size={13} />} {t('agent.restart')}
                </button>
                <button className="btn" disabled={anyBusy} onClick={() => void act('stop', () => AgentLifecycle(serverId, 'stop'))}>
                  <Power size={13} /> {t('agent.stop')}
                </button>
              </>
            ) : (
              <button className="btn btn--primary" disabled={anyBusy} onClick={() => void act('start', () => AgentLifecycle(serverId, 'start'))}>
                {busy === 'start' ? <span className="spinner" /> : <Power size={13} />} {t('agent.start')}
              </button>
            )}

            <button
              className="btn"
              disabled={anyBusy}
              onClick={() => void act('logs', () => AgentLogs(serverId, 200), (out) => setLogs(out))}
            >
              {busy === 'logs' ? <span className="spinner" /> : <FileText size={13} />} {t('agent.logs')}
            </button>

            {/* Hanya muncul kalau versi yang dibundel terbukti lebih baru dari
                yang terpasang — bukan sekadar berbeda. */}
            {status.updateAvailable && (
              <button className="btn btn--primary" disabled={anyBusy} onClick={() => void act('update', () => UpdateAgent(serverId))}>
                {busy === 'update' ? <span className="spinner" /> : <Download size={13} />}{' '}
                {t('agent.updateTo', { version: status.bundledVersion ?? '' })}
              </button>
            )}

            <button className="btn btn--danger" disabled={anyBusy} onClick={() => void handleUninstall()}>
              <Trash2 size={13} /> {t('agent.uninstall')}
            </button>
          </div>

          {/* Aksinya SELALU tersedia selama agent terpasang, bukan hanya saat
              pengelolaan-diri mati. Menyalakannya dan benar-benar bisa
              menyambung adalah dua hal berbeda — mis. known_hosts agent belum
              terisi — dan tanpa jalan memasang ulang kuncinya, satu-satunya
              pemulihan adalah copot-pasang yang menghapus token bot dan
              allowlist pairing. Operasinya idempoten: kunci lama diganti,
              bukan ditumpuk. */}
          <div className="agent-selfmanage">
            {status.selfManaged ? (
              <p className="agent-hint">{t('agent.selfRepairHint')}</p>
            ) : (
              <>
                <p className="fw-notice">
                  <TriangleAlert size={13} /> {t('agent.selfOff')}
                </p>
                <p className="agent-hint">{t('agent.manageSelfHint')}</p>
              </>
            )}
            <button
              className={status.selfManaged ? 'btn' : 'btn btn--primary'}
              disabled={anyBusy}
              onClick={() => void act('selfmanage', () => EnableAgentSelfManagement(serverId))}
            >
              {busy === 'selfmanage' ? <span className="spinner" /> : <ServerCog size={13} />}{' '}
              {status.selfManaged ? t('agent.selfRepair') : t('agent.enableSelf')}
            </button>
          </div>

          {logs !== null && (
            <pre className="agent-logs" onDoubleClick={() => setLogs(null)}>
              {logs}
            </pre>
          )}
        </section>
      )}

      {/* ---- server tertaut ---- */}
      {running && (
        <section className="agent-section">
          <h3>
            <Link2 size={14} /> {t('agent.linked')}
          </h3>
          <p className="agent-hint">{t('agent.linkedHint')}</p>

          {linkError && (
            <>
              <p className="fw-notice fw-notice--warn">
                <TriangleAlert size={13} /> {linkError}
              </p>
              <button className="btn" disabled={anyBusy} onClick={() => void loadLinked()}>
                <RotateCw size={13} /> {t('common.retry')}
              </button>
            </>
          )}

          {linked !== null && (
            <>
          <ul className="agent-users">
            {linked.map((l) => (
              <li key={l.id}>
                <span>
                  {/* Status koneksi menurut AGENT, bukan menurut app ini —
                      keduanya bisa berbeda, dan yang menentukan isi balasan
                      bot adalah pandangan agent. */}
                  <span title={l.error || l.connection || ''}>{connDot(l.connection)}</span>{' '}
                  <strong>{l.name}</strong> <code>{l.host}</code>
                  {l.isSelf && <em> — {t('agent.thisServer')}</em>}
                  {l.error && <em className="agent-linkerr"> — {l.error}</em>}
                </span>
                {!l.isSelf && (
                  <button
                    className="btn btn--ghost"
                    disabled={anyBusy}
                    onClick={() =>
                      void act('unlink', () => UnlinkServerFromAgent(serverId, l.id), (r) => setLinked(r))
                    }
                  >
                    <Trash2 size={12} /> {t('agent.unlink')}
                  </button>
                )}
              </li>
            ))}
          </ul>

          {/* Hanya server yang belum tertaut yang ditawarkan — menampilkan yang
              sudah ada cuma mengundang pengiriman ulang tanpa maksud. */}
          {(() => {
            const available = allServers.filter(
              (srv) => srv.id !== serverId && !linked.some((l) => l.id === srv.id),
            );
            if (available.length === 0) {
              return <p className="agent-hint">{t('agent.noneToLink')}</p>;
            }
            return (
              <>
                <h4 className="agent-subhead">{t('agent.pickServers')}</h4>
                {available.map((srv) => (
                  <label key={srv.id} className="agent-check">
                    <input
                      type="checkbox"
                      checked={picked.has(srv.id)}
                      onChange={(e) =>
                        setPicked((prev) => {
                          const next = new Set(prev);
                          if (e.target.checked) next.add(srv.id);
                          else next.delete(srv.id);
                          return next;
                        })
                      }
                    />
                    <span>
                      {srv.name} <code>{srv.host}</code>
                    </span>
                  </label>
                ))}
                <div className="agent-actions">
                  <button
                    className="btn btn--primary"
                    disabled={anyBusy || picked.size === 0}
                    onClick={() =>
                      void act(
                        'link',
                        () =>
                          LinkServersToAgent({
                            serverId,
                            targetIds: Array.from(picked),
                          } as agentctl.LinkRequest),
                        (r) => {
                          setLinked(r);
                          setPicked(new Set());
                        },
                      )
                    }
                  >
                    {busy === 'link' ? <span className="spinner" /> : <Link2 size={13} />}{' '}
                    {t('agent.linkSelected')}
                  </button>
                </div>
                <p className="agent-hint">{t('agent.linkWarning')}</p>
              </>
            );
          })()}
            </>
          )}
        </section>
      )}

      {/* ---- bot telegram ---- */}
      {installed && (
        <section className="agent-section">
          <h3>
            <Bot size={14} /> {t('agent.telegram')}
          </h3>

          {status.telegram.botUsername && (
            <p className="fw-notice">
              <CircleCheck size={13} /> {t('agent.botConnected', { bot: status.telegram.botUsername })}
            </p>
          )}
          {status.telegram.lastError && (
            <p className="fw-notice fw-notice--warn">
              <TriangleAlert size={13} /> {status.telegram.lastError}
            </p>
          )}

          <label className="agent-field">
            <span>{t('agent.botToken')}</span>
            <input
              type="password"
              value={token}
              placeholder={status.telegram.configured ? t('agent.tokenStored') : t('agent.tokenPlaceholder')}
              onChange={(e) => {
                setToken(e.target.value);
                setTokenTest(null);
              }}
            />
          </label>
          <p className="agent-hint">{t('agent.tokenHint')}</p>

          <div className="agent-actions">
            <button
              className="btn"
              disabled={anyBusy || (!token && !status.telegram.configured)}
              onClick={() => void act('test', () => TestAgentTelegramToken(serverId, token), (r) => setTokenTest(r))}
            >
              {busy === 'test' ? <span className="spinner" /> : <Bot size={13} />} {t('agent.testToken')}
            </button>
            <button
              className="btn btn--primary"
              disabled={anyBusy || (!token && !status.telegram.configured)}
              onClick={() =>
                void act('savetg', () =>
                  ConfigureAgentTelegram({ serverId, enabled: true, token } as agentctl.TelegramRequest),
                ).then(() => setToken(''))
              }
            >
              {busy === 'savetg' ? <span className="spinner" /> : null} {t('agent.saveAndRestart')}
            </button>
            {status.telegram.enabled && (
              <button
                className="btn"
                disabled={anyBusy}
                onClick={() =>
                  void act('offtg', () =>
                    ConfigureAgentTelegram({ serverId, enabled: false, token: '' } as agentctl.TelegramRequest),
                  )
                }
              >
                {t('agent.disableBot')}
              </button>
            )}
          </div>

          {tokenTest && (
            <p className={`fw-notice${tokenTest.ok ? '' : ' fw-notice--warn'}`}>
              {tokenTest.ok ? <CircleCheck size={13} /> : <TriangleAlert size={13} />}{' '}
              {tokenTest.ok ? t('agent.botConnected', { bot: tokenTest.botUsername ?? '' }) : tokenTest.message}
            </p>
          )}

          {/* ---- siapa yang boleh ---- */}
          <h4 className="agent-subhead">{t('agent.allowedUsers')}</h4>
          {status.telegram.allowedUsers.length === 0 ? (
            <p className="agent-hint">{t('agent.noAllowedUsers')}</p>
          ) : (
            <ul className="agent-users">
              {status.telegram.allowedUsers.map((id) => (
                <li key={id}>
                  <code>{id}</code>
                  <button className="btn btn--ghost" disabled={anyBusy} onClick={() => void handleRevoke(id)}>
                    <Trash2 size={12} /> {t('agent.revoke')}
                  </button>
                </li>
              ))}
            </ul>
          )}

          <button
            className="btn"
            disabled={anyBusy || !status.telegram.enabled || !running}
            onClick={() => void act('pair', () => CreateAgentPairingCode(serverId), (pc) => setPairing(pc))}
          >
            {busy === 'pair' ? <span className="spinner" /> : null} {t('agent.newPairingCode')}
          </button>

          {pairing && (
            <div className="agent-pairing">
              <code className="agent-pairing__code">{pairing.code}</code>
              <button
                className="btn btn--ghost"
                onClick={() => void navigator.clipboard?.writeText(pairing.code)}
                title={t('common.copy')}
              >
                <Copy size={13} />
              </button>
              <span className="agent-hint">
                {t('agent.pairingHint', { minutes: String(Math.max(1, Math.round(pairing.ttlSecond / 60))) })}
              </span>
            </div>
          )}

          <p className="agent-hint">{t('agent.commandsHint')}</p>
        </section>
      )}
    </div>
  );
}

// connDot memakai simbol yang sama dengan yang dipakai bot, supaya apa yang
// terlihat di sini dan di Telegram tidak pernah tampak bertentangan.
function connDot(conn?: string) {
  switch (conn) {
    case 'online':
      return '🟢';
    case 'reconnecting':
      return '🟡';
    case 'offline':
      return '🔴';
    default:
      return '⚪';
  }
}

function phaseKey(phase: string): MessageKey {
  switch (phase) {
    case 'upload':
      return 'agent.phaseUpload';
    case 'verify':
      return 'agent.phaseVerify';
    case 'install':
      return 'agent.phaseInstall';
    case 'health':
      return 'agent.phaseHealth';
    case 'done':
      return 'agent.phaseDone';
    default:
      return 'agent.phasePrepare';
  }
}

function stateKey(state: string): MessageKey {
  switch (state) {
    case 'running':
      return 'agent.stateRunning';
    case 'stopped':
      return 'agent.stateStopped';
    default:
      return 'agent.stateNotInstalled';
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
