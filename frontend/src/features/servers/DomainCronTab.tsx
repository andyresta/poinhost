import { useEffect, useRef, useState } from 'react';
import { X, RotateCw, FileText, Pencil, Play, Pause, Trash2 } from 'lucide-react';
import {
  ListWebsiteCronJobs,
  CreateWebsiteCronJob,
  UpdateWebsiteCronJob,
  ToggleWebsiteCronJob,
  DeleteWebsiteCronJob,
  ReadWebsiteCronLog,
} from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { useConfirm } from '../../components/ConfirmDialog';
import { useT } from '../../i18n';

const PRESETS: { labelKey: string; value: string }[] = [
  // labelKey, bukan teks jadi — diterjemahkan saat render supaya ikut
  // berubah ketika bahasa diganti tanpa reload.
  { labelKey: 'cron.preset.everyMinute', value: '* * * * *' },
  { labelKey: 'cron.preset.every5', value: '*/5 * * * *' },
  { labelKey: 'cron.preset.every15', value: '*/15 * * * *' },
  { labelKey: 'cron.preset.every30', value: '*/30 * * * *' },
  { labelKey: 'cron.preset.hourly', value: '0 * * * *' },
  { labelKey: 'cron.preset.daily', value: '0 2 * * *' },
  { labelKey: 'cron.preset.weekly', value: '0 2 * * 1' },
  { labelKey: 'cron.preset.monthly', value: '0 2 1 * *' },
];

// Bentuk form lokal (plain object) — dipisah dari class website.CronJobRequest
// karena state React yang di-spread berulang (`{...form, x}`) tidak boleh
// bergantung pada method (convertValues) milik instance class; instance
// CronJobRequest yang sesungguhnya baru dibuat sesaat sebelum dikirim ke API.
interface CronFormState {
  id?: string;
  taskType: string;
  schedule: string;
  command?: string;
  url?: string;
  method?: string;
  payload?: string;
  headers?: website.CronHeader[];
  description?: string;
  enabled: boolean;
}

function emptyForm(): CronFormState {
  return { taskType: 'command', schedule: '0 * * * *', method: 'GET', enabled: true, headers: [] };
}

function formFromJob(job: website.CronJobInfo): CronFormState {
  return {
    id: job.id,
    taskType: job.taskType,
    schedule: job.schedule,
    command: job.command,
    url: job.url,
    method: job.method,
    payload: job.payload,
    headers: job.headers,
    description: job.description,
    enabled: job.enabled,
  };
}

function CronLogModal({ serverId, domain, jobId, onClose }: { serverId: string; domain: string; jobId: string; onClose: () => void }) {
  const t = useT();
  const [content, setContent] = useState('');
  const [exists, setExists] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    ReadWebsiteCronLog(new website.CronLogRequest({ serverId, domain, id: jobId, lines: 500 }))
      .then((res) => {
        setExists(res.exists);
        setContent(res.content);
      })
      .catch((e) => setError(String(e)));
  }, [serverId, domain, jobId]);

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--editor">
        <div className="modal-card__header">
          <h2>{t('cron.jobLog')}</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body modal-card__body--editor">
          {error && <p className="overview__error">{error}</p>}
          {!exists && <p className="workspace__placeholder">{t('cron.noLog')}</p>}
          {exists && <pre className="docker-engine__log">{content}</pre>}
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

export function DomainCronTab({ serverId, domain }: { serverId: string; domain: string }) {
  const t = useT();
  const confirm = useConfirm();
  const [list, setList] = useState<website.CronListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [form, setForm] = useState<CronFormState | null>(null);
  const [logJobId, setLogJobId] = useState<string | null>(null);
  const formError = useRef<string | null>(null);

  async function load() {
    try {
      setList(await ListWebsiteCronJobs(serverId, domain));
    } catch (e) {
      setError(String(e));
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, domain]);

  async function handleToggle(job: website.CronJobInfo) {
    setBusy(job.id);
    setError(null);
    try {
      await ToggleWebsiteCronJob(new website.CronToggleRequest({ serverId, domain, id: job.id, enabled: !job.enabled }));
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function handleDelete(job: website.CronJobInfo) {
    const ok = await confirm({
      title: t('cron.deleteTitle'),
      message: (
        <>
          Hapus job <strong>{job.description || job.actionSummary}</strong>?
        </>
      ),
      confirmLabel: t('common.delete'),
      danger: true,
    });
    if (!ok) return;
    setBusy(job.id);
    setError(null);
    try {
      await DeleteWebsiteCronJob(serverId, domain, job.id);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function handleSubmit() {
    if (!form) return;
    formError.current = null;
    setBusy('__form__');
    try {
      const req = new website.CronJobRequest({ ...form, serverId, domain });
      if (form.id) await UpdateWebsiteCronJob(req);
      else await CreateWebsiteCronJob(req);
      setForm(null);
      await load();
    } catch (e) {
      formError.current = String(e);
      setForm({ ...form }); // force rerender to show error
    } finally {
      setBusy(null);
    }
  }

  if (!list) return <p className="workspace__placeholder">{t('common.loading')}</p>;

  return (
    <div>
      {error && <p className="overview__error">{error}</p>}

      <div className="files-panel__toolbar">
        <button className="btn btn--sm" onClick={() => void load()}>
          <RotateCw size={13} /> {t('common.refresh')}
        </button>
        <button className="btn btn--sm btn--primary" onClick={() => setForm(emptyForm())}>
          + {t('cron.newJob')}
        </button>
        <span className="docker-panel__version">
          {t('cron.summary', { active: String(list.active), inactive: String(list.inactive) })}
        </span>
      </div>

      <table className="files-panel__table">
        <thead>
          <tr>
            <th>{t('cron.colSchedule')}</th>
            <th>{t('cron.colAction')}</th>
            <th>{t('cron.colStatus')}</th>
            <th className="files-panel__col-actions" />
          </tr>
        </thead>
        <tbody>
          {list.jobs.map((job) => (
            <tr key={job.id}>
              <td>{job.scheduleText}</td>
              <td style={{ maxWidth: 240, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={job.description}>
                {job.description || job.actionSummary}
              </td>
              <td>
                <span className={`docker-badge ${job.enabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
                  {job.enabled ? t('cron.active') : t('cron.inactive')}
                </span>
              </td>
              <td className="files-panel__row-actions">
                <button title={t('cron.jobLog')} onClick={() => setLogJobId(job.id)}>
                  <FileText size={14} />
                </button>
                <button title={t('cron.editJob')} onClick={() => setForm(formFromJob(job))}>
                  <Pencil size={14} />
                </button>
                <button title={job.enabled ? t('cron.disable') : t('cron.enable')} disabled={busy === job.id} onClick={() => void handleToggle(job)}>
                  {busy === job.id ? <span className="spinner" /> : job.enabled ? <Pause size={14} /> : <Play size={14} />}
                </button>
                <button title={t('common.delete')} disabled={busy === job.id} onClick={() => void handleDelete(job)}>
                  {busy === job.id ? <span className="spinner" /> : <Trash2 size={14} />}
                </button>
              </td>
            </tr>
          ))}
          {list.jobs.length === 0 && (
            <tr>
              <td colSpan={4} className="files-panel__empty">
                Belum ada job cron untuk domain ini.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      {form && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && setForm(null)}>
          <div className="modal-card">
            <div className="modal-card__header">
              <h2>{form.id ? t('cron.editJob') : t('cron.newJob')}</h2>
              <button className="modal-card__close" onClick={() => setForm(null)}>
                <X size={18} />
              </button>
            </div>
            <div className="modal-card__body">
              <div className="segmented">
                <button
                  className={`segmented__item${form.taskType === 'command' ? ' segmented__item--active' : ''}`}
                  onClick={() => setForm({ ...form, taskType: 'command' })}
                >
                  {t('cron.shellCommand')}
                </button>
                <button
                  className={`segmented__item${form.taskType === 'http' ? ' segmented__item--active' : ''}`}
                  onClick={() => setForm({ ...form, taskType: 'http' })}
                >
                  {t('cron.httpCall')}
                </button>
              </div>

              <label className="form-field">
                <span>{t('cron.description')}</span>
                <input value={form.description ?? ''} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </label>

              {form.taskType === 'command' ? (
                <label className="form-field">
                  <span>{t('cron.command')}</span>
                  <input
                    placeholder="php artisan schedule:run"
                    value={form.command ?? ''}
                    onChange={(e) => setForm({ ...form, command: e.target.value })}
                  />
                </label>
              ) : (
                <>
                  <label className="form-field">
                    <span>URL</span>
                    <input placeholder="https://contoh.com/cron/tick" value={form.url ?? ''} onChange={(e) => setForm({ ...form, url: e.target.value })} />
                  </label>
                  <label className="form-field">
                    <span>{t('cron.method')}</span>
                    <select value={form.method ?? 'GET'} onChange={(e) => setForm({ ...form, method: e.target.value })}>
                      <option value="GET">GET</option>
                      <option value="POST">POST</option>
                    </select>
                  </label>
                </>
              )}

              <label className="form-field">
                <span>{t('cron.schedule')}</span>
                <input value={form.schedule} onChange={(e) => setForm({ ...form, schedule: e.target.value })} />
              </label>
              <div className="files-panel__toolbar">
                {PRESETS.map((p) => (
                  <button key={p.value} className="btn btn--sm" onClick={() => setForm({ ...form, schedule: p.value })}>
                    {t(p.labelKey as 'cron.preset.hourly')}
                  </button>
                ))}
              </div>

              <label className="form-check">
                <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
                <span>{t('cron.enabled')}</span>
              </label>

              {formError.current && <p className="overview__error">{formError.current}</p>}
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => setForm(null)}>
                {t('common.cancel')}
              </button>
              <button className="btn btn--primary" disabled={busy === '__form__'} onClick={() => void handleSubmit()}>
                {busy === '__form__' && <span className="spinner" />} {busy === '__form__' ? t('cron.saving') : t('common.save')}
              </button>
            </div>
          </div>
        </div>
      )}

      {logJobId && <CronLogModal serverId={serverId} domain={domain} jobId={logJobId} onClose={() => setLogJobId(null)} />}
    </div>
  );
}
