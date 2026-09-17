import { useEffect, useState } from 'react';
import { ArrowRight, Play, X, ChevronDown, ChevronUp } from 'lucide-react';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { XferStart, XferCancel } from '../../../wailsjs/go/main/App';
import { filexfer } from '../../../wailsjs/go/models';
import { useTabsStore } from '../../store/tabs';
import { useT } from '../../i18n';
import type { MessageKey } from '../../i18n/messages';
import { RemoteBrowser, formatBytes } from './RemoteBrowser';

const RUNNING = new Set(['queued', 'scanning', 'running']);

// Status dari backend dipetakan ke key terjemahan lewat tabel eksplisit,
// bukan template string: dengan begitu TypeScript ikut menjamin setiap
// status punya terjemahan, dan status baru di Go akan ketahuan saat
// kompilasi, bukan muncul sebagai key mentah di layar.
const STATUS_LABEL: Record<string, MessageKey> = {
  queued: 'migration.status.queued',
  scanning: 'migration.status.scanning',
  running: 'migration.status.running',
  done: 'migration.status.done',
  failed: 'migration.status.failed',
  canceled: 'migration.status.canceled',
};

const PATH_LABEL: Record<string, MessageKey> = {
  pending: 'migration.path.pending',
  running: 'migration.path.running',
  done: 'migration.path.done',
  failed: 'migration.path.failed',
};

// Panel "Migrasi Files": pilih item di server asal, tentukan folder di
// server tujuan, lalu jalankan.
//
// Datanya mengalir LANGSUNG dari server asal ke server tujuan — `tar` di
// sisi asal menulis ke stdout, disambung di memori proses ini, masuk ke
// stdin `tar` di sisi tujuan. Tidak ada file perantara di komputer ini,
// dan tidak ada tahap "unduh dulu, baru unggah" yang menggandakan waktu
// transfer.
export function FileTransferPanel() {
  const t = useT();
  const serverList = useTabsStore((s) => s.servers);

  const [srcServerId, setSrcServerId] = useState('');
  const [srcPath, setSrcPath] = useState('/');
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const [dstServerId, setDstServerId] = useState('');
  const [dstPath, setDstPath] = useState('/');

  const [compress, setCompress] = useState(true);
  const [exclude, setExclude] = useState('');
  const [showOptions, setShowOptions] = useState(false);

  const [job, setJob] = useState<filexfer.Progress | null>(null);
  const [destReload, setDestReload] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Berlangganan event progress dibongkar-pasang mengikuti job yang sedang
  // ditonton; tanpa ini, memulai transfer kedua akan meninggalkan listener
  // job lama yang masih menimpa state dengan snapshot usang.
  const jobId = job?.jobId ?? '';
  useEffect(() => {
    if (!jobId) return;
    const off = EventsOn(`xfer:progress:${jobId}`, (raw: unknown) => {
      setJob(filexfer.Progress.createFrom(raw));
    });
    return off;
  }, [jobId]);

  // Begitu job berhenti, muat ulang sisi tujuan: file yang baru masuk harus
  // langsung terlihat, bukan menunggu user menekan Refresh sendiri untuk
  // memastikan transfernya benar-benar mendarat.
  const jobStatus = job?.status ?? '';
  useEffect(() => {
    if (jobStatus && !RUNNING.has(jobStatus)) {
      setDestReload((n) => n + 1);
    }
  }, [jobStatus]);

  function toggle(entry: filexfer.DirEntry) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(entry.path)) next.delete(entry.path);
      else next.add(entry.path);
      return next;
    });
  }

  // Pindah folder di sisi sumber tidak menghapus pilihan: memilih beberapa
  // folder dari lokasi yang berbeda adalah kasus yang wajar saat migrasi
  // (mis. /var/www dan /etc/nginx sekaligus).
  function handleSrcServerChange(id: string) {
    setSrcServerId(id);
    setSelected(new Set());
  }

  const running = job !== null && RUNNING.has(job.status);
  const canStart =
    !busy && !running && srcServerId !== '' && dstServerId !== '' && selected.size > 0;

  async function handleStart() {
    setBusy(true);
    setError(null);
    try {
      const progress = await XferStart(
        filexfer.StartRequest.createFrom({
          sourceServerId: srcServerId,
          sourcePaths: Array.from(selected),
          destServerId: dstServerId,
          destPath: dstPath,
          exclude: exclude
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean),
          compress,
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
      setJob(await XferCancel(job.jobId));
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <div className="mig-panel">
      <div className="mig-panel__sides">
        <div className="mig-panel__side">
          <div className="mig-panel__side-title">{t('migration.source')}</div>
          <RemoteBrowser
            serverList={serverList}
            serverId={srcServerId}
            onServerChange={handleSrcServerChange}
            path={srcPath}
            onPathChange={setSrcPath}
            selectable
            selected={selected}
            onToggle={toggle}
            disabled={running}
          />
        </div>

        <div className="mig-panel__arrow">
          <ArrowRight size={18} />
        </div>

        <div className="mig-panel__side">
          <div className="mig-panel__side-title">{t('migration.dest')}</div>
          <RemoteBrowser
            serverList={serverList}
            serverId={dstServerId}
            onServerChange={setDstServerId}
            path={dstPath}
            onPathChange={setDstPath}
            selectable={false}
            disabled={running}
            reloadToken={destReload}
          />
          {/* Folder tujuan boleh diketik langsung, tidak harus ditelusuri:
              folder yang belum ada tidak bisa dibuka di penjelajah, padahal
              memindahkan ke folder baru justru kasus yang lazim. Backend
              membuatnya lebih dulu (mkdir -p) di perintah yang sama. */}
          <input
            className="mig-panel__destpath"
            type="text"
            value={dstPath}
            disabled={running}
            spellCheck={false}
            onChange={(e) => setDstPath(e.target.value)}
          />
        </div>
      </div>

      <div className="mig-panel__bar">
        <div className="mig-panel__summary">
          {t('migration.summary', { count: selected.size, dest: dstPath })}
        </div>
        <button className="btn btn--sm" onClick={() => setShowOptions((v) => !v)}>
          {t('migration.options')}
          {showOptions ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
        </button>
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
            <Play size={13} /> {t('migration.start')}
          </button>
        )}
      </div>

      {showOptions && (
        <div className="mig-panel__options">
          <label className="mig-panel__opt">
            <input
              type="checkbox"
              checked={compress}
              disabled={running}
              onChange={(e) => setCompress(e.target.checked)}
            />
            {t('migration.compress')}
          </label>
          <label className="mig-panel__opt mig-panel__opt--grow">
            {t('migration.exclude')}
            <input
              type="text"
              value={exclude}
              disabled={running}
              placeholder="node_modules, *.log"
              onChange={(e) => setExclude(e.target.value)}
            />
          </label>
        </div>
      )}

      {error && <div className="mig-panel__error">{error}</div>}

      {job && (
        <div className="mig-panel__progress">
          <div className="mig-panel__progress-head">
            <span className={`mig-panel__status mig-panel__status--${job.status}`}>
              {t(STATUS_LABEL[job.status] ?? 'migration.status.queued')}
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
              {job.paths.map((p) => (
                <tr key={p.path}>
                  <td className="mig-panel__path-name">{p.path}</td>
                  <td className={`mig-panel__path-state mig-panel__path-state--${p.status}`}>
                    {t(PATH_LABEL[p.status] ?? 'migration.path.pending')}
                  </td>
                  <td className="mig-panel__path-error">{p.error ?? ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
