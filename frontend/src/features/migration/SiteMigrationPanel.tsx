import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, Play, X, Info, RefreshCw, Search, AlertTriangle, XCircle, CheckCircle2, ListChecks } from 'lucide-react';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { ListWebsites, SiteXferPreview, SiteXferStart, SiteXferCancel, SiteXferRepairDBUsers } from '../../../wailsjs/go/main/App';
import { sitexfer, website } from '../../../wailsjs/go/models';
import { useTabsStore } from '../../store/tabs';
import { useT } from '../../i18n';
import type { MessageKey } from '../../i18n/messages';
import { formatBytes } from './RemoteBrowser';

const RUNNING = new Set(['queued', 'inspecting', 'running', 'verifying']);

const STATUS_LABEL: Record<string, MessageKey> = {
  queued: 'migration.db.status.queued',
  inspecting: 'migration.site.status.inspecting',
  running: 'migration.db.status.running',
  verifying: 'migration.db.status.verifying',
  done: 'migration.db.status.done',
  partial: 'migration.db.status.partial',
  failed: 'migration.db.status.failed',
  canceled: 'migration.db.status.canceled',
};

const ITEM_LABEL: Record<string, MessageKey> = {
  pending: 'migration.site.item.pending',
  running: 'migration.site.item.running',
  done: 'migration.site.item.done',
  failed: 'migration.site.item.failed',
  skipped: 'migration.site.item.skipped',
};

const KIND_LABEL: Record<string, MessageKey> = {
  dbuser: 'migration.site.kind.dbuser',
  database: 'migration.site.kind.database',
  sftp: 'migration.site.kind.sftp',
  files: 'migration.site.kind.files',
  vhost: 'migration.site.kind.vhost',
  cron: 'migration.site.kind.cron',
};

const ACTION_LABEL: Record<string, MessageKey> = {
  create: 'migration.site.action.create',
  'skip-exists': 'migration.site.action.skipExists',
  'skip-global': 'migration.site.action.skipGlobal',
  'needs-password': 'migration.site.action.needsPassword',
};

const userKey = (u: { username: string; host?: string }) => `${u.username}@${u.host ?? ''}`;

// Urutan tampil: domain induk, langsung diikuti subdomain-subdomainnya.
function orderDomains(domains: website.DomainInfo[]): website.DomainInfo[] {
  const parents = domains.filter((d) => !d.isSubdomain).sort((a, b) => a.domain.localeCompare(b.domain));
  const children = domains.filter((d) => d.isSubdomain);
  const out: website.DomainInfo[] = [];
  for (const p of parents) {
    out.push(p);
    out.push(...children.filter((c) => c.parent === p.domain).sort((a, b) => a.domain.localeCompare(b.domain)));
  }
  // Subdomain yang induknya tidak ada di server ini tetap ditampilkan.
  out.push(...children.filter((c) => !parents.some((p) => p.domain === c.parent)));
  return out;
}

// Panel "Migrasi Website": pilih domain dan/atau subdomain di server asal
// (masing-masing berdiri sendiri), lalu pindahkan file, vhost, database +
// user-nya, akun SFTP, dan cron ke server tujuan. PHP & SSL disiapkan
// ulang manual di tujuan — lihat internal/modules/sitexfer.
//
// Alurnya dua langkah — "Periksa" dulu, baru "Migrasikan" — karena migrasi
// ini menulis ke banyak tempat di server tujuan sekaligus (nginx, database,
// user Linux, cron). Preview menunjukkan persis apa yang akan dibuat dan
// bentrok apa yang menghalangi, SEBELUM ada satu pun yang berubah.
export function SiteMigrationPanel() {
  const t = useT();
  const serverList = useTabsStore((s) => s.servers);

  const [srcServerId, setSrcServerId] = useState('');
  const [dstServerId, setDstServerId] = useState('');
  const [domains, setDomains] = useState<website.DomainInfo[]>([]);
  const [loadingDomains, setLoadingDomains] = useState(false);
  const [domainsError, setDomainsError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const [includeDatabases, setIncludeDatabases] = useState(true);
  const [includeSftp, setIncludeSftp] = useState(true);
  const [includeCron, setIncludeCron] = useState(true);
  const [compress, setCompress] = useState(true);

  const [plan, setPlan] = useState<sitexfer.Plan | null>(null);
  const [planKey, setPlanKey] = useState('');
  const [checking, setChecking] = useState(false);

  const [job, setJob] = useState<sitexfer.Progress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Password asli untuk user DB yang hash-nya tidak portabel (MySQL 8 →
  // MariaDB). SENGAJA tidak ikut requestKey: mengetik password tidak
  // membuat hasil Periksa basi, dan password tidak pernah ikut di-JSON-kan
  // untuk perbandingan.
  const [passwords, setPasswords] = useState<Record<string, string>>({});
  const [repair, setRepair] = useState<sitexfer.RepairResult | null>(null);
  const [repairing, setRepairing] = useState(false);

  async function loadDomains(serverId: string) {
    setDomains([]);
    setSelected(new Set());
    setDomainsError(null);
    if (!serverId) return;
    setLoadingDomains(true);
    try {
      const res = await ListWebsites(serverId);
      setDomains(orderDomains(res.domains ?? []));
    } catch (e) {
      setDomainsError(String(e));
    } finally {
      setLoadingDomains(false);
    }
  }

  useEffect(() => {
    void loadDomains(srcServerId);
  }, [srcServerId]);

  const jobId = job?.jobId ?? '';
  useEffect(() => {
    if (!jobId) return;
    const off = EventsOn(`sitexfer:progress:${jobId}`, (raw: unknown) => {
      setJob(sitexfer.Progress.createFrom(raw));
    });
    return off;
  }, [jobId]);

  const request = useMemo(
    () =>
      sitexfer.StartRequest.createFrom({
        sourceServerId: srcServerId,
        destServerId: dstServerId,
        domains: Array.from(selected).sort(),
        includeDatabases,
        includeSftp,
        includeCron,
        compress,
      }),
    [srcServerId, dstServerId, selected, includeDatabases, includeSftp, includeCron, compress],
  );
  const requestKey = JSON.stringify(request);
  const planFresh = plan !== null && planKey === requestKey;

  const running = job !== null && RUNNING.has(job.status);
  const ready = srcServerId !== '' && dstServerId !== '' && selected.size > 0;
  const canCheck = ready && !busy && !checking && !running;
  const needsPw = plan?.dbUsers.filter((u) => u.action === 'needs-password') ?? [];
  const pwMissing = needsPw.some((u) => !passwords[userKey(u)]);
  const canStart = canCheck && planFresh && plan?.canStart === true && !pwMissing;
  const repairable = plan?.dbUsers.some((u) => u.action === 'create' || u.action === 'needs-password') ?? false;
  const withPasswords = () => sitexfer.StartRequest.createFrom({ ...request, dbUserPasswords: passwords });

  function toggle(domain: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(domain)) next.delete(domain);
      else next.add(domain);
      return next;
    });
  }

  async function handleCheck() {
    setChecking(true);
    setError(null);
    try {
      setRepair(null);
      const p = await SiteXferPreview(request);
      setPlan(p);
      setPlanKey(requestKey);
    } catch (e) {
      setError(String(e));
      setPlan(null);
    } finally {
      setChecking(false);
    }
  }

  async function handleStart() {
    setBusy(true);
    setError(null);
    try {
      setJob(await SiteXferStart(withPasswords()));
      setPasswords({});
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleRepair() {
    setRepairing(true);
    setError(null);
    try {
      setRepair(await SiteXferRepairDBUsers(withPasswords()));
      setPasswords({});
    } catch (e) {
      setError(String(e));
    } finally {
      setRepairing(false);
    }
  }

  async function handleCancel() {
    if (!job) return;
    try {
      setJob(await SiteXferCancel(job.jobId));
    } catch (e) {
      setError(String(e));
    }
  }

  const dstServerName = serverList.find((s) => s.id === dstServerId)?.name ?? '—';
  const blocking = plan?.problems.filter((p) => p.blocking) ?? [];
  const warnings = plan?.problems.filter((p) => !p.blocking) ?? [];

  return (
    <div className="mig-panel">
      <div className="mig-panel__notice">
        <Info size={13} />
        <span>{t('migration.site.notice')}</span>
      </div>

      <div className="mig-panel__sides">
        <div className="mig-panel__side">
          <div className="mig-panel__side-title">{t('migration.source')}</div>
          <div className="mig-browser">
            <div className="mig-browser__head">
              <select
                className="mig-browser__server"
                value={srcServerId}
                disabled={running}
                onChange={(e) => setSrcServerId(e.target.value)}
              >
                <option value="">{t('migration.pickServer')}</option>
                {serverList.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
              <button
                className="btn btn--sm"
                title={t('common.refresh')}
                disabled={!srcServerId || loadingDomains || running}
                onClick={() => void loadDomains(srcServerId)}
              >
                <RefreshCw size={13} />
              </button>
            </div>
            <div className="mig-browser__list">
              <div className="mig-browser__scroll">
                {!srcServerId && <div className="mig-browser__empty">{t('migration.pickServerFirst')}</div>}
                {srcServerId && domainsError && <div className="mig-browser__error">{domainsError}</div>}
                {srcServerId && !domainsError && !loadingDomains && domains.length === 0 && (
                  <div className="mig-browser__empty">{t('migration.site.noDomains')}</div>
                )}
                {domains.length > 0 && (
                  <table className="mig-browser__table">
                    <tbody>
                      {domains.map((d) => (
                        <tr
                          key={d.domain}
                          className={selected.has(d.domain) ? 'mig-browser__row--selected' : ''}
                          onClick={() => !running && toggle(d.domain)}
                        >
                          <td className="mig-browser__col-check">
                            <input type="checkbox" checked={selected.has(d.domain)} disabled={running} readOnly />
                          </td>
                          <td className={`mig-browser__name${d.isSubdomain ? ' mig-site__sub' : ''}`}>
                            <span className="mig-browser__ellipsis">{d.domain}</span>
                          </td>
                          <td className="mig-site__badges">
                            {d.phpVersion && <span className="mig-site__badge">PHP {d.phpVersion}</span>}
                            {d.sslEnabled && <span className="mig-site__badge">SSL</span>}
                            {(d.proxyTarget || (d.proxyRules?.length ?? 0) > 0) && (
                              <span className="mig-site__badge">Proxy</span>
                            )}
                            {!d.enabled && (
                              <span className="mig-site__badge mig-site__badge--muted">{t('migration.site.disabled')}</span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
              {loadingDomains && <div className="mig-browser__loading">{t('common.loading')}</div>}
            </div>
          </div>
        </div>

        <div className="mig-panel__arrow">
          <ArrowRight size={18} />
        </div>

        <div className="mig-panel__side">
          <div className="mig-panel__side-title">{t('migration.dest')}</div>
          <div className="mig-browser mig-panel__destserver">
            <div className="mig-browser__head">
              <select
                className="mig-browser__server"
                value={dstServerId}
                disabled={running}
                onChange={(e) => setDstServerId(e.target.value)}
              >
                <option value="">{t('migration.pickServer')}</option>
                {serverList
                  .filter((s) => s.id !== srcServerId)
                  .map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
              </select>
            </div>
          </div>
        </div>
      </div>

      <div className="mig-panel__options">
        <label className="mig-panel__opt">
          <input type="checkbox" checked={includeDatabases} disabled={running} onChange={(e) => setIncludeDatabases(e.target.checked)} />
          {t('migration.site.opt.databases')}
        </label>
        <label className="mig-panel__opt">
          <input type="checkbox" checked={includeSftp} disabled={running} onChange={(e) => setIncludeSftp(e.target.checked)} />
          {t('migration.site.opt.sftp')}
        </label>
        <label className="mig-panel__opt">
          <input type="checkbox" checked={includeCron} disabled={running} onChange={(e) => setIncludeCron(e.target.checked)} />
          {t('migration.site.opt.cron')}
        </label>
        <label className="mig-panel__opt">
          <input type="checkbox" checked={compress} disabled={running} onChange={(e) => setCompress(e.target.checked)} />
          {t('migration.compress')}
        </label>
      </div>

      <div className="mig-panel__bar">
        <div className="mig-panel__summary">
          {t('migration.site.summary', { count: String(selected.size), dest: dstServerName })}
        </div>
        {running ? (
          <button className="btn btn--sm btn--danger" onClick={() => void handleCancel()}>
            <X size={13} /> {t('common.cancel')}
          </button>
        ) : (
          <>
            <button className="btn btn--sm" disabled={!canCheck} onClick={() => void handleCheck()}>
              <Search size={13} /> {checking ? t('migration.site.checking') : t('migration.site.check')}
            </button>
            <button className="btn btn--primary btn--sm" disabled={!canStart} onClick={() => void handleStart()}>
              <Play size={13} /> {t('migration.site.start')}
            </button>
          </>
        )}
      </div>

      {error && <div className="mig-panel__error">{error}</div>}

      {plan && !job && (
        <div className="mig-site__plan">
          {!planFresh && (
            <div className="mig-site__problem">
              <AlertTriangle size={13} /> {t('migration.site.plan.stale')}
            </div>
          )}
          {blocking.map((p, i) => (
            <div key={`b${i}`} className="mig-site__problem mig-site__problem--blocking">
              <XCircle size={13} /> {p.message}
            </div>
          ))}
          {warnings.map((p, i) => (
            <div key={`w${i}`} className="mig-site__problem">
              <AlertTriangle size={13} /> {p.message}
            </div>
          ))}
          {planFresh && plan.canStart && (
            <div className="mig-site__problem mig-site__problem--ok">
              <CheckCircle2 size={13} /> {t('migration.site.plan.ready', { size: formatBytes(plan.totalBytes) })}
            </div>
          )}

          <div className="mig-site__section">{t('migration.site.plan.folders')}</div>
          <table className="mig-panel__paths">
            <tbody>
              {plan.paths.map((p) => (
                <tr key={p.path}>
                  <td className="mig-panel__path-name">
                    {p.path}
                    {p.excludes.length > 0 && (
                      <div className="mig-site__muted">{t('migration.site.plan.excluding', { paths: p.excludes.join(', ') })}</div>
                    )}
                  </td>
                  <td className="mig-panel__path-verify">{t('migration.site.plan.files', { count: String(p.files) })}</td>
                  <td className="mig-panel__path-verify">{formatBytes(p.bytes)}</td>
                </tr>
              ))}
            </tbody>
          </table>

          {plan.databases.length > 0 && (
            <>
              <div className="mig-site__section">{t('migration.site.plan.databases')}</div>
              <table className="mig-panel__paths">
                <tbody>
                  {plan.databases.map((d) => (
                    <tr key={`${d.engine}:${d.name}`}>
                      <td className="mig-panel__path-name">
                        <span className="mig-panel__item-kind">{d.engine === 'postgresql' ? 'PostgreSQL' : 'MySQL'}</span>
                        {d.name}
                      </td>
                      <td className="mig-panel__path-error">{d.domains.join(', ')}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}

          {plan.dbUsers.length > 0 && (
            <>
              <div className="mig-site__section">{t('migration.site.plan.dbUsers')}</div>
              <table className="mig-panel__paths">
                <tbody>
                  {plan.dbUsers.map((u) => (
                    <tr key={`${u.engine}:${u.username}@${u.host}`}>
                      <td className="mig-panel__path-name">
                        {u.username}
                        {u.host ? `@${u.host}` : ''}
                      </td>
                      <td className={`mig-panel__path-state mig-site__action--${u.action}`}>
                        {t(ACTION_LABEL[u.action] ?? 'migration.site.action.create')}
                      </td>
                      <td className="mig-panel__path-error">
                        {u.note || u.databases.join(', ')}
                        {u.action === 'needs-password' && (
                          <input
                            type="password"
                            className="mig-site__pw"
                            autoComplete="off"
                            placeholder={t('migration.site.passwordPlaceholder')}
                            value={passwords[userKey(u)] ?? ''}
                            onChange={(e) => setPasswords((p) => ({ ...p, [userKey(u)]: e.target.value }))}
                          />
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}

          {repairable && planFresh && (
            <div className="mig-site__repair">
              <span className="mig-site__muted">{t('migration.site.repairHint')}</span>
              <button className="btn btn--sm" disabled={pwMissing || repairing || running} onClick={() => void handleRepair()}>
                {repairing ? t('migration.site.checking') : t('migration.site.repair')}
              </button>
            </div>
          )}
          {repair && (
            <table className="mig-panel__paths">
              <tbody>
                {repair.items.map((it) => (
                  <tr key={it.key}>
                    <td className="mig-panel__path-name">{it.label}</td>
                    <td className={`mig-panel__path-state mig-panel__path-state--${it.status}`}>
                      {t(ITEM_LABEL[it.status] ?? 'migration.site.item.pending')}
                    </td>
                    <td className="mig-panel__path-error">{it.error || it.warning || ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {plan.sftp.length > 0 && (
            <>
              <div className="mig-site__section">{t('migration.site.plan.sftp')}</div>
              <table className="mig-panel__paths">
                <tbody>
                  {plan.sftp.map((a) => (
                    <tr key={a.username}>
                      <td className="mig-panel__path-name">{a.username}</td>
                      <td className="mig-panel__path-verify">{a.domain}</td>
                      <td className="mig-panel__path-error">{a.note || ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}

          {plan.cronJobs > 0 && (
            <div className="mig-site__section">{t('migration.site.plan.cron', { count: String(plan.cronJobs) })}</div>
          )}
        </div>
      )}

      {job && (
        <div className="mig-panel__progress">
          <div className="mig-panel__progress-head">
            <span className={`mig-panel__status mig-panel__status--${job.status}`}>
              {t(STATUS_LABEL[job.status] ?? 'migration.db.status.queued')}
            </span>
            <span className="mig-panel__bytes">
              {formatBytes(job.doneBytes)}
              {job.totalBytes > 0 && ` / ${formatBytes(job.totalBytes)}`}
            </span>
            {job.message && <span className="mig-panel__message">{job.message}</span>}
          </div>
          <div className="mig-panel__meter">
            <div
              className="mig-panel__meter-fill"
              style={{ width: `${job.totalBytes > 0 ? job.percent : running ? 100 : 0}%` }}
            />
          </div>
          <table className="mig-panel__paths">
            <tbody>
              {job.items.map((it) => (
                <tr key={it.key}>
                  <td className="mig-panel__path-name">
                    {KIND_LABEL[it.kind] && <span className="mig-panel__item-kind">{t(KIND_LABEL[it.kind])}</span>}
                    {it.label}
                  </td>
                  <td className={`mig-panel__path-state mig-panel__path-state--${it.status}`}>
                    {t(ITEM_LABEL[it.status] ?? 'migration.site.item.pending')}
                  </td>
                  <td className="mig-panel__path-error">{it.error || it.warning || it.detail || ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {job.report.length > 0 && (
            <div className="mig-site__report">
              <div className="mig-site__section">
                <ListChecks size={13} /> {t('migration.site.report')}
              </div>
              <ul>
                {job.report.map((line, i) => (
                  <li key={i}>{line}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
