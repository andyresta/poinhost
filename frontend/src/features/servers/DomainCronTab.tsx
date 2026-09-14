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

const PRESETS: { label: string; value: string }[] = [
  { label: 'Tiap menit', value: '* * * * *' },
  { label: 'Tiap 5 menit', value: '*/5 * * * *' },
  { label: 'Tiap 15 menit', value: '*/15 * * * *' },
  { label: 'Tiap 30 menit', value: '*/30 * * * *' },
  { label: 'Tiap jam', value: '0 * * * *' },
  { label: 'Harian 02:00', value: '0 2 * * *' },
  { label: 'Mingguan (Senin 02:00)', value: '0 2 * * 1' },
  { label: 'Bulanan (tgl 1, 02:00)', value: '0 2 1 * *' },
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
          <h2>Log Job</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body modal-card__body--editor">
          {error && <p className="overview__error">{error}</p>}
          {!exists && <p className="workspace__placeholder">Belum ada log — job ini belum pernah jalan.</p>}
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
    if (!confirm(`Hapus job "${job.description || job.actionSummary}"?`)) return;
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

  if (!list) return <p className="workspace__placeholder">Memuat…</p>;

  return (
    <div>
      {error && <p className="overview__error">{error}</p>}

      <div className="files-panel__toolbar">
        <button className="btn btn--sm" onClick={() => void load()}>
          <RotateCw size={13} /> Refresh
        </button>
        <button className="btn btn--sm btn--primary" onClick={() => setForm(emptyForm())}>
          + Job Baru
        </button>
        <span className="docker-panel__version">
          {list.active} aktif · {list.inactive} nonaktif
        </span>
      </div>

      <table className="files-panel__table">
        <thead>
          <tr>
            <th>Jadwal</th>
            <th>Aksi</th>
            <th>Status</th>
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
                  {job.enabled ? 'Aktif' : 'Nonaktif'}
                </span>
              </td>
              <td className="files-panel__row-actions">
                <button title="Log" onClick={() => setLogJobId(job.id)}>
                  <FileText size={14} />
                </button>
                <button title="Edit" onClick={() => setForm(formFromJob(job))}>
                  <Pencil size={14} />
                </button>
                <button title={job.enabled ? 'Nonaktifkan' : 'Aktifkan'} disabled={busy === job.id} onClick={() => void handleToggle(job)}>
                  {busy === job.id ? <span className="spinner" /> : job.enabled ? <Pause size={14} /> : <Play size={14} />}
                </button>
                <button title="Hapus" disabled={busy === job.id} onClick={() => void handleDelete(job)}>
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
              <h2>{form.id ? 'Edit Job' : 'Job Baru'}</h2>
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
                  Perintah shell
                </button>
                <button
                  className={`segmented__item${form.taskType === 'http' ? ' segmented__item--active' : ''}`}
                  onClick={() => setForm({ ...form, taskType: 'http' })}
                >
                  Panggilan HTTP
                </button>
              </div>

              <label className="form-field">
                <span>Deskripsi (opsional)</span>
                <input value={form.description ?? ''} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </label>

              {form.taskType === 'command' ? (
                <label className="form-field">
                  <span>Perintah (dijalankan dari document root domain)</span>
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
                    <span>Method</span>
                    <select value={form.method ?? 'GET'} onChange={(e) => setForm({ ...form, method: e.target.value })}>
                      <option value="GET">GET</option>
                      <option value="POST">POST</option>
                    </select>
                  </label>
                </>
              )}

              <label className="form-field">
                <span>Jadwal (5 field cron)</span>
                <input value={form.schedule} onChange={(e) => setForm({ ...form, schedule: e.target.value })} />
              </label>
              <div className="files-panel__toolbar">
                {PRESETS.map((p) => (
                  <button key={p.value} className="btn btn--sm" onClick={() => setForm({ ...form, schedule: p.value })}>
                    {p.label}
                  </button>
                ))}
              </div>

              <label className="form-check">
                <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
                <span>Aktif</span>
              </label>

              {formError.current && <p className="overview__error">{formError.current}</p>}
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => setForm(null)}>
                Batal
              </button>
              <button className="btn btn--primary" disabled={busy === '__form__'} onClick={() => void handleSubmit()}>
                {busy === '__form__' && <span className="spinner" />} {busy === '__form__' ? 'Menyimpan…' : 'Simpan'}
              </button>
            </div>
          </div>
        </div>
      )}

      {logJobId && <CronLogModal serverId={serverId} domain={domain} jobId={logJobId} onClose={() => setLogJobId(null)} />}
    </div>
  );
}
