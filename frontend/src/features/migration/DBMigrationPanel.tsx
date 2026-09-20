import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, Play, X, Info } from 'lucide-react';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { DbXferStart, DbXferCancel } from '../../../wailsjs/go/main/App';
import { dbxfer } from '../../../wailsjs/go/models';
import { useTabsStore } from '../../store/tabs';
import { useT } from '../../i18n';
import type { MessageKey } from '../../i18n/messages';
import { DatabasePicker, type DBSelection } from './DatabasePicker';
import { formatBytes } from './RemoteBrowser';

const RUNNING = new Set(['queued', 'inspecting', 'running', 'verifying']);

const STATUS_LABEL: Record<string, MessageKey> = {
  queued: 'migration.db.status.queued',
  inspecting: 'migration.db.status.inspecting',
  running: 'migration.db.status.running',
  verifying: 'migration.db.status.verifying',
  done: 'migration.db.status.done',
  partial: 'migration.db.status.partial',
  failed: 'migration.db.status.failed',
  canceled: 'migration.db.status.canceled',
};

const ITEM_LABEL: Record<string, MessageKey> = {
  pending: 'migration.db.item.pending',
  running: 'migration.db.item.running',
  done: 'migration.db.item.done',
  failed: 'migration.db.item.failed',
};

const VERIFY_LABEL: Record<string, MessageKey> = {
  match: 'migration.db.verify.match',
  mismatch: 'migration.db.verify.mismatch',
  skipped: 'migration.db.verify.skipped',
};

// Panel "Migrasi Database": pilih satu atau lebih database (dan opsional
// subset tabelnya) di server asal, pilih server tujuan, lalu migrasikan.
//
// Beda dari migrasi Docker (satu container per job): migrasi database
// mendukung BANYAK database sekaligus dalam satu job, jalan paralel lewat
// worker pool di backend — keputusan yang dikunci eksplisit bareng user,
// karena migrasi beberapa database aplikasi sekaligus adalah kasus nyata
// yang berguna (lihat ARCHITECTURE.md).
export function DBMigrationPanel() {
  const t = useT();
  const serverList = useTabsStore((s) => s.servers);

  const [srcServerId, setSrcServerId] = useState('');
  const [engine, setEngine] = useState('mysql');
  const [selected, setSelected] = useState<Map<string, DBSelection>>(new Map());

  const [dstServerId, setDstServerId] = useState('');
  const [destDatabase, setDestDatabase] = useState('');

  const [job, setJob] = useState<dbxfer.Progress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const jobId = job?.jobId ?? '';
  useEffect(() => {
    if (!jobId) return;
    const off = EventsOn(`dbxfer:progress:${jobId}`, (raw: unknown) => {
      setJob(dbxfer.Progress.createFrom(raw));
    });
    return off;
  }, [jobId]);

  const running = job !== null && RUNNING.has(job.status);
  const canStart = !busy && !running && srcServerId !== '' && dstServerId !== '' && selected.size > 0;
  const singleSelection = selected.size === 1 ? Array.from(selected.keys())[0] : null;

  const summary = useMemo(() => {
    const names = Array.from(selected.keys());
    if (names.length === 0) return '—';
    if (names.length <= 2) return names.join(', ');
    return `${names.slice(0, 2).join(', ')} (+${names.length - 2})`;
  }, [selected]);

  function handleSrcServerChange(id: string) {
    setSrcServerId(id);
    setSelected(new Map());
  }

  async function handleStart() {
    setBusy(true);
    setError(null);
    try {
      const items = Array.from(selected.entries()).map(([database, sel]) =>
        dbxfer.ItemSelection.createFrom({
          database,
          allTables: sel.allTables,
          tables: sel.allTables ? [] : Array.from(sel.tables),
          destDatabase: singleSelection === database ? destDatabase.trim() : '',
        }),
      );
      const progress = await DbXferStart(
        dbxfer.StartRequest.createFrom({
          sourceServerId: srcServerId,
          destServerId: dstServerId,
          engine,
          items,
        }),
      );
      setJob(progress);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleCancel() {
    if (!job) return;
    try {
      setJob(await DbXferCancel(job.jobId));
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <div className="mig-panel">
      <div className="mig-panel__notice">
        <Info size={13} />
        <span>{t('migration.db.notice')}</span>
      </div>

      <div className="mig-panel__sides">
        <div className="mig-panel__side">
          <div className="mig-panel__side-title">{t('migration.source')}</div>
          <DatabasePicker
            serverList={serverList}
            serverId={srcServerId}
            onServerChange={handleSrcServerChange}
            engine={engine}
            onEngineChange={setEngine}
            selected={selected}
            onSelectedChange={setSelected}
            disabled={running}
          />
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
          {singleSelection && (
            <input
              className="mig-panel__destpath"
              type="text"
              placeholder={t('migration.db.destNamePlaceholder', { name: singleSelection })}
              value={destDatabase}
              disabled={running}
              spellCheck={false}
              onChange={(e) => setDestDatabase(e.target.value)}
            />
          )}
        </div>
      </div>

      <div className="mig-panel__bar">
        <div className="mig-panel__summary">{t('migration.db.summary', { names: summary })}</div>
        {running ? (
          <button className="btn btn--sm btn--danger" onClick={() => void handleCancel()}>
            <X size={13} /> {t('common.cancel')}
          </button>
        ) : (
          <button
            className="btn btn--primary btn--sm"
            disabled={!canStart}
            onClick={() => void handleStart()}
          >
            <Play size={13} /> {t('migration.db.start')}
          </button>
        )}
      </div>

      {error && <div className="mig-panel__error">{error}</div>}

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
                <tr key={it.database}>
                  <td className="mig-panel__path-name">
                    {it.database}
                    {it.destDatabase !== it.database && ` → ${it.destDatabase}`}
                    {it.tableCount > 0 && ` (${it.tableCount})`}
                  </td>
                  <td className={`mig-panel__path-state mig-panel__path-state--${it.status}`}>
                    {t(ITEM_LABEL[it.status] ?? 'migration.db.item.pending')}
                  </td>
                  <td className="mig-panel__path-verify">
                    {it.verify ? t(VERIFY_LABEL[it.verify] ?? 'migration.db.verify.skipped') : ''}
                  </td>
                  <td className="mig-panel__path-error">
                    {it.error || it.warning || it.verifyDetail || ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
