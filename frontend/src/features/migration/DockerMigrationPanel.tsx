import { useEffect, useState } from 'react';
import { ArrowRight, Play, X, Info, CheckCircle2, XCircle, AlertTriangle } from 'lucide-react';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { DockerXferStart, DockerXferCancel, DockerXferComposeInfo } from '../../../wailsjs/go/main/App';
import { docker, dockerxfer } from '../../../wailsjs/go/models';
import { useTabsStore } from '../../store/tabs';
import { useT } from '../../i18n';
import type { MessageKey } from '../../i18n/messages';
import { ContainerPicker } from './ContainerPicker';
import { formatBytes } from './RemoteBrowser';

const RUNNING = new Set(['queued', 'inspecting', 'running', 'verifying']);

const STATUS_LABEL: Record<string, MessageKey> = {
  queued: 'migration.docker.status.queued',
  inspecting: 'migration.docker.status.inspecting',
  running: 'migration.docker.status.running',
  verifying: 'migration.docker.status.verifying',
  done: 'migration.docker.status.done',
  failed: 'migration.docker.status.failed',
  canceled: 'migration.docker.status.canceled',
};

const ITEM_LABEL: Record<string, MessageKey> = {
  pending: 'migration.docker.item.pending',
  running: 'migration.docker.item.running',
  done: 'migration.docker.item.done',
  failed: 'migration.docker.item.failed',
  skipped: 'migration.docker.item.skipped',
};

const VERIFY_LABEL: Record<string, MessageKey> = {
  match: 'migration.docker.verify.match',
  mismatch: 'migration.docker.verify.mismatch',
  skipped: 'migration.docker.verify.skipped',
};

const KIND_LABEL: Record<string, MessageKey> = {
  bind: 'migration.docker.kind.bind',
  volume: 'migration.docker.kind.volume',
  image: 'migration.docker.kind.image',
  workdir: 'migration.docker.kind.workdir',
};

// Compose file yang tidak berada di dalam direktori proyek (dipakai lewat
// `-f` dari tempat lain) tidak ikut tersalin bersama workdir.
function filesOutside(info: docker.ComposeMigrationInfo): string[] {
  const dir = info.workingDir.replace(/\/+$/, '');
  return (info.configFiles ?? []).filter((f) => f !== dir && !f.startsWith(dir + '/'));
}

// Item label datang dari backend sebagai "kind:value" (mis. "bind:/var/www",
// "volume:app_data", "image:nginx:1.25") — dipecah di sini murni untuk
// tampilan, backend tidak perlu tahu apa-apa soal bagaimana ini dirender.
function splitItemLabel(label: string): { kind: string; value: string } {
  const i = label.indexOf(':');
  if (i < 0) return { kind: '', value: label };
  return { kind: label.slice(0, i), value: label.slice(i + 1) };
}

// Panel "Migrasi Docker": pilih SATU container di server asal, pilih server
// tujuan, lalu migrasikan — konfigurasi + mount (bind/volume) + image
// disalin, container dibuat & dinyalakan di tujuan.
//
// Beda mendasar dari FileTransferPanel: unit yang dipindah bukan
// sekumpulan path bebas, tapi satu container Docker utuh, jadi sisi tujuan
// tidak perlu dijelajah (namanya sudah ditentukan dari konfigurasi
// container itu sendiri) — cukup pilih server tujuannya.
export function DockerMigrationPanel() {
  const t = useT();
  const serverList = useTabsStore((s) => s.servers);

  const [srcServerId, setSrcServerId] = useState('');
  const [srcContainerId, setSrcContainerId] = useState('');
  const [srcContainerName, setSrcContainerName] = useState('');

  const [dstServerId, setDstServerId] = useState('');

  const [compress, setCompress] = useState(true);
  const [copyWorkdir, setCopyWorkdir] = useState(false);

  const [compose, setCompose] = useState<docker.ComposeMigrationInfo | null>(null);
  const [composeLoading, setComposeLoading] = useState(false);
  const [composeError, setComposeError] = useState<string | null>(null);

  // Info proyek compose dibaca begitu container sumber dipilih, supaya opsi
  // "salin direktori proyek" hanya bisa dicentang kalau memang ada
  // direktorinya, dan container lain di proyek yang sama terlihat SEBELUM
  // migrasi dimulai.
  useEffect(() => {
    setCompose(null);
    setComposeError(null);
    setCopyWorkdir(false);
    if (!srcServerId || !srcContainerId) return;
    let stale = false;
    setComposeLoading(true);
    DockerXferComposeInfo(srcServerId, srcContainerId)
      .then((info) => {
        if (!stale) setCompose(info);
      })
      .catch((e) => {
        if (!stale) setComposeError(String(e));
      })
      .finally(() => {
        if (!stale) setComposeLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [srcServerId, srcContainerId]);

  const [job, setJob] = useState<dockerxfer.Progress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const jobId = job?.jobId ?? '';
  useEffect(() => {
    if (!jobId) return;
    const off = EventsOn(`dockerxfer:progress:${jobId}`, (raw: unknown) => {
      setJob(dockerxfer.Progress.createFrom(raw));
    });
    return off;
  }, [jobId]);

  const running = job !== null && RUNNING.has(job.status);
  const canStart = !busy && !running && srcServerId !== '' && srcContainerId !== '' && dstServerId !== '';

  function handleContainerChange(id: string, name: string) {
    setSrcContainerId(id);
    setSrcContainerName(name);
  }

  async function handleStart() {
    setBusy(true);
    setError(null);
    try {
      const progress = await DockerXferStart(
        dockerxfer.StartRequest.createFrom({
          sourceServerId: srcServerId,
          containerId: srcContainerId,
          destServerId: dstServerId,
          compress,
          copyWorkdir,
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
      setJob(await DockerXferCancel(job.jobId));
    } catch (e) {
      setError(String(e));
    }
  }

  const dstServerName = serverList.find((s) => s.id === dstServerId)?.name ?? '';

  return (
    <div className="mig-panel">
      <div className="mig-panel__notice">
        <Info size={13} />
        <span>{t('migration.docker.notice')}</span>
      </div>

      <div className="mig-panel__sides">
        <div className="mig-panel__side">
          <div className="mig-panel__side-title">{t('migration.source')}</div>
          <ContainerPicker
            serverList={serverList}
            serverId={srcServerId}
            onServerChange={setSrcServerId}
            containerId={srcContainerId}
            onContainerChange={handleContainerChange}
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
        </div>
      </div>

      {srcContainerId !== '' && (
        <div className="mig-panel__options mig-panel__compose">
          {composeLoading ? (
            <span className="mig-panel__compose-note">{t('migration.docker.composeLoading')}</span>
          ) : composeError ? (
            <span className="mig-panel__compose-note mig-panel__compose-note--warn">{composeError}</span>
          ) : compose && !compose.managed ? (
            <span className="mig-panel__compose-note">{t('migration.docker.notCompose')}</span>
          ) : compose ? (
            <>
              <label className="mig-panel__opt">
                <input
                  type="checkbox"
                  checked={copyWorkdir}
                  disabled={running || !compose.workingDirExists}
                  onChange={(e) => setCopyWorkdir(e.target.checked)}
                />
                {t('migration.docker.copyWorkdir')}
              </label>
              <span className="mig-panel__compose-note">
                {compose.workingDirExists
                  ? t('migration.docker.workdirHint', {
                      dir: compose.workingDir,
                      size: formatBytes(compose.workingDirBytes),
                    })
                  : t('migration.docker.workdirMissing', { dir: compose.workingDir || '—' })}
              </span>
              {copyWorkdir && filesOutside(compose).length > 0 && (
                <span className="mig-panel__compose-note mig-panel__compose-note--warn">
                  <AlertTriangle size={12} />
                  {t('migration.docker.configOutside', { files: filesOutside(compose).join(', ') })}
                </span>
              )}
              {compose.siblings.length > 0 && (
                <span className="mig-panel__compose-note mig-panel__compose-note--warn">
                  <AlertTriangle size={12} />
                  {t('migration.docker.siblings', {
                    project: compose.project,
                    names: compose.siblings.join(', '),
                  })}
                </span>
              )}
            </>
          ) : null}
        </div>
      )}

      <div className="mig-panel__bar">
        <div className="mig-panel__summary">
          {t('migration.docker.summary', {
            name: srcContainerName || '—',
            dest: dstServerName || '—',
          })}
        </div>
        <label className="mig-panel__opt">
          <input
            type="checkbox"
            checked={compress}
            disabled={running}
            onChange={(e) => setCompress(e.target.checked)}
          />
          {t('migration.compress')}
        </label>
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
            <Play size={13} /> {t('migration.docker.start')}
          </button>
        )}
      </div>

      {error && <div className="mig-panel__error">{error}</div>}

      {job && (
        <div className="mig-panel__progress">
          <div className="mig-panel__progress-head">
            <span className={`mig-panel__status mig-panel__status--${job.status}`}>
              {t(STATUS_LABEL[job.status] ?? 'migration.docker.status.queued')}
            </span>
            <span className="mig-panel__bytes">
              {formatBytes(job.doneBytes)}
              {job.totalBytes > 0 && ` / ${formatBytes(job.totalBytes)}`}
            </span>
            {job.containerRunning !== undefined && job.containerRunning !== null && (
              <span
                className={`mig-panel__container-status mig-panel__container-status--${job.containerRunning ? 'ok' : 'bad'}`}
              >
                {job.containerRunning ? <CheckCircle2 size={13} /> : <XCircle size={13} />}
                {t(
                  job.containerRunning
                    ? 'migration.docker.containerRunning'
                    : 'migration.docker.containerNotRunning',
                )}
              </span>
            )}
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
              {job.items.map((it) => {
                const { kind, value } = splitItemLabel(it.label);
                return (
                  <tr key={it.label}>
                    <td className="mig-panel__path-name">
                      {kind && KIND_LABEL[kind] && (
                        <span className="mig-panel__item-kind">{t(KIND_LABEL[kind])}</span>
                      )}
                      {value}
                    </td>
                    <td className={`mig-panel__path-state mig-panel__path-state--${it.status}`}>
                      {t(ITEM_LABEL[it.status] ?? 'migration.docker.item.pending')}
                    </td>
                    <td className="mig-panel__path-verify">
                      {it.verify ? t(VERIFY_LABEL[it.verify] ?? 'migration.docker.verify.skipped') : ''}
                    </td>
                    <td className="mig-panel__path-error">
                      {it.error || it.warning || it.verifyDetail || ''}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
