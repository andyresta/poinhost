import { useEffect, useState } from 'react';
import { RotateCw, Square, SquareTerminal, ChartColumn, Play, FileText, Settings, Trash2, FileCog } from 'lucide-react';
import {
  ListDockerContainers,
  DetectDockerEngine,
  StartDockerContainer,
  StopDockerContainer,
  RestartDockerContainer,
  RemoveDockerContainer,
  GetComposeStatus,
  ApplyComposeConfig,
} from '../../../wailsjs/go/main/App';
import { docker } from '../../../wailsjs/go/models';
import { useConfirm } from '../../components/ConfirmDialog';
import { useT } from '../../i18n';
import { EngineInstallWizard } from './EngineInstallWizard';
import { NetworksPanel } from './NetworksPanel';
import { ContainerLogsModal } from './ContainerLogsModal';
import { ContainerStatsModal } from './ContainerStatsModal';
import { ContainerExecModal } from './ContainerExecModal';
import { RecreateContainerModal } from './RecreateContainerModal';

const SUB_TABS = [
  { key: 'containers', label: 'Containers' },
  { key: 'networks', label: 'Networks' },
  { key: 'images', label: 'Images' },
  { key: 'volumes', label: 'Volumes' },
  { key: 'compose', label: 'Compose' },
] as const;
type SubTab = (typeof SUB_TABS)[number]['key'];

// Container list di-refresh berkala selagi tab Containers aktif — murah
// karena ListContainers di-cache 8 detik di sisi backend (docker.Service),
// jadi polling di sini tidak berarti `docker ps` baru tiap kali.
const REFRESH_INTERVAL_MS = 10_000;

function stateBadgeClass(state: string): string {
  switch (state) {
    case 'running':
      return 'docker-badge docker-badge--running';
    case 'paused':
      return 'docker-badge docker-badge--paused';
    case 'restarting':
      return 'docker-badge docker-badge--restarting';
    default:
      return 'docker-badge docker-badge--stopped';
  }
}

// Modul Docker untuk SATU tab. Scope-nya SENGAJA mengikuti homepoin apa
// adanya (lihat ARCHITECTURE.md §Docker): Containers & Networks berfungsi
// penuh, Images/Volumes/Compose adalah placeholder jujur — karena di
// homepoin sendiri ketiganya belum pernah diimplementasikan (cuma halaman
// "coming soon" tanpa backend).
export function DockerPanel({ tabId, serverId }: { tabId: string; serverId: string }) {
  const confirm = useConfirm();
  const t = useT();
  const [subTab, setSubTab] = useState<SubTab>('containers');
  const [engine, setEngine] = useState<docker.EngineStatus | null>(null);
  const [engineError, setEngineError] = useState<string | null>(null);
  const [containers, setContainers] = useState<docker.ContainerInfo[]>([]);
  const [loading, setLoading] = useState(true);
  // Lihat catatan yang sama di WebsitePanel: refresh manual butuh flag
  // sendiri supaya ada spinner dan daftar lama tidak ikut hilang.
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const [logsTarget, setLogsTarget] = useState<docker.ContainerInfo | null>(null);
  const [statsTarget, setStatsTarget] = useState<docker.ContainerInfo | null>(null);
  const [execTarget, setExecTarget] = useState<docker.ContainerInfo | null>(null);
  const [configTarget, setConfigTarget] = useState<docker.ContainerInfo | null>(null);

  async function loadEngine() {
    setEngineError(null);
    try {
      const st = await DetectDockerEngine(serverId);
      setEngine(st);
      return st;
    } catch (e) {
      setEngineError(String(e));
      return null;
    }
  }

  async function loadContainers() {
    setRefreshing(true);
    try {
      const res = await ListDockerContainers(serverId);
      setContainers(res.containers);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void (async () => {
      const st = await loadEngine();
      if (cancelled) return;
      if (st?.installed && st.active) {
        await loadContainers();
      } else {
        setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  useEffect(() => {
    if (subTab !== 'containers' || !engine?.installed || !engine.active) return;
    const id = setInterval(() => void loadContainers(), REFRESH_INTERVAL_MS);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subTab, serverId, engine?.installed, engine?.active]);

  async function runAction(containerId: string, fn: () => Promise<void>) {
    setBusyId(containerId);
    setError(null);
    try {
      await fn();
      await loadContainers();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusyId(null);
    }
  }

  async function handleRemove(c: docker.ContainerInfo) {
    const ok = await confirm({
      title: 'Hapus container',
      message: (
        <>
          Hapus container <strong>{c.name}</strong>?
        </>
      ),
      detail: 'Container akan dipaksa berhenti lalu dihapus.',
      confirmLabel: 'Hapus',
      danger: true,
    });
    if (!ok) return;
    await runAction(c.id, () => RemoveDockerContainer(serverId, c.id));
  }

  // Menerapkan ulang compose. Drift DICEK dan DITAMPILKAN lebih dulu, supaya
  // user tahu apakah ada yang tertinggal — bukan sekadar menekan tombol dan
  // berharap. `docker compose up -d` biasa tidak cukup: kalau container-nya
  // sudah ada, perintah itu cuma men-start-nya tanpa menerapkan konfigurasi
  // baru. Itulah yang membuat setelan compose bisa tertinggal diam-diam.
  async function handleApplyCompose(c: docker.ContainerInfo) {
    setBusyId(c.id);
    setError(null);
    try {
      const info = await GetComposeStatus(serverId, c.id);
      if (!info.managed) {
        setError(t('docker.composeNotManaged', { name: c.name }));
        return;
      }
      const keadaan = info.unknown
        ? t('docker.composeUnknown', { reason: info.message || '-' })
        : info.drift
          ? t('docker.composeDrift')
          : t('docker.composeInSync');
      const ok = await confirm({
        title: t('docker.composeApply'),
        message: t('docker.composeConfirm', { service: info.service }),
        detail: `${keadaan}

${t('docker.composeDetail', { file: info.configFiles || info.workingDir })}`,
        confirmLabel: t('docker.composeApply'),
        danger: true,
      });
      if (!ok) return;
      const after = await ApplyComposeConfig(serverId, c.id);
      if (after.drift) {
        setError(t('docker.composeStillDrift'));
      }
      await loadContainers();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="docker-panel">
      <nav className="docker-panel__subnav">
        {SUB_TABS.map((tab) => (
          <button
            key={tab.key}
            className={`docker-panel__subnav-item${subTab === tab.key ? ' docker-panel__subnav-item--active' : ''}`}
            onClick={() => setSubTab(tab.key)}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {subTab === 'containers' && (
        <div className="files-panel">
          {engineError && <p className="overview__error">{engineError}</p>}

          {engine && (!engine.installed || !engine.active) && (
            <EngineInstallWizard
              serverId={serverId}
              status={engine}
              onChange={() => {
                void loadEngine().then(async (st) => {
                  if (st?.active) await loadContainers();
                });
              }}
            />
          )}

          {engine?.installed && engine.active && (
            <>
              <div className="files-panel__toolbar">
                <button className="btn btn--sm" disabled={refreshing} onClick={() => void loadContainers()}>
                  {refreshing ? <span className="spinner" /> : <RotateCw size={13} />} {t('common.refresh')}
                </button>
                {engine.version && <span className="docker-panel__version">Docker {engine.version}</span>}
              </div>

              {error && <p className="overview__error">{error}</p>}

              <div className="files-panel__table-wrap">
                <div className="files-panel__table-scroll">
                <table className="files-panel__table">
                  <thead>
                    <tr>
                      <th>Nama</th>
                      <th>Image</th>
                      <th>Status</th>
                      <th>Port</th>
                      <th className="files-panel__col-actions" />
                    </tr>
                  </thead>
                  <tbody>
                    {containers.map((c) => (
                      <tr key={c.id}>
                        <td className="files-panel__name">{c.name}</td>
                        <td>
                          <div className="files-panel__ellipsis" title={c.image}>
                            {c.image}
                          </div>
                        </td>
                        <td>
                          <span className={stateBadgeClass(c.state)}>{c.status || c.state}</span>
                        </td>
                        <td>
                          <div className="files-panel__ellipsis" title={c.ports}>
                            {c.ports || '—'}
                          </div>
                        </td>
                        <td className="files-panel__row-actions">
                          {c.state === 'running' ? (
                            <>
                              <button title="Stop" disabled={busyId === c.id} onClick={() => void runAction(c.id, () => StopDockerContainer(serverId, c.id))}>
                                {busyId === c.id ? <span className="spinner" /> : <Square size={14} />}
                              </button>
                              <button
                                title="Restart"
                                disabled={busyId === c.id}
                                onClick={() => void runAction(c.id, () => RestartDockerContainer(serverId, c.id))}
                              >
                                {busyId === c.id ? <span className="spinner" /> : <RotateCw size={14} />}
                              </button>
                              <button title="Exec" onClick={() => setExecTarget(c)}>
                                <SquareTerminal size={14} />
                              </button>
                              <button title="Stats" onClick={() => setStatsTarget(c)}>
                                <ChartColumn size={14} />
                              </button>
                            </>
                          ) : (
                            <button title="Start" disabled={busyId === c.id} onClick={() => void runAction(c.id, () => StartDockerContainer(serverId, c.id))}>
                              {busyId === c.id ? <span className="spinner" /> : <Play size={14} />}
                            </button>
                          )}
                          <button title="Log" onClick={() => setLogsTarget(c)}>
                            <FileText size={14} />
                          </button>
                          <button title="Konfigurasi" onClick={() => setConfigTarget(c)}>
                            <Settings size={14} />
                          </button>
                          <button
                            title={t('docker.composeApply')}
                            disabled={busyId === c.id}
                            onClick={() => void handleApplyCompose(c)}
                          >
                            <FileCog size={14} />
                          </button>
                          <button title="Hapus" disabled={busyId === c.id} onClick={() => void handleRemove(c)}>
                            {busyId === c.id ? <span className="spinner" /> : <Trash2 size={14} />}
                          </button>
                        </td>
                      </tr>
                    ))}
                    {!loading && containers.length === 0 && (
                      <tr>
                        <td colSpan={5} className="files-panel__empty">
                          Tidak ada container.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
                </div>
                {loading && <div className="files-panel__loading">{t('common.loading')}</div>}
              </div>
            </>
          )}
        </div>
      )}

      {subTab === 'networks' && <NetworksPanel serverId={serverId} />}

      {(subTab === 'images' || subTab === 'volumes' || subTab === 'compose') && (
        <p className="workspace__placeholder">
          Submenu <code>{subTab}</code> belum tersedia — sama seperti di homepoin, belum ada
          implementasinya di sana (baru halaman "coming soon"), jadi belum ada yang bisa di-porting.
        </p>
      )}

      {logsTarget && (
        <ContainerLogsModal serverId={serverId} containerId={logsTarget.id} name={logsTarget.name} onClose={() => setLogsTarget(null)} />
      )}
      {statsTarget && (
        <ContainerStatsModal serverId={serverId} containerId={statsTarget.id} name={statsTarget.name} onClose={() => setStatsTarget(null)} />
      )}
      {execTarget && <ContainerExecModal tabId={tabId} containerId={execTarget.id} name={execTarget.name} onClose={() => setExecTarget(null)} />}
      {configTarget && (
        <RecreateContainerModal
          serverId={serverId}
          containerId={configTarget.id}
          name={configTarget.name}
          onClose={() => setConfigTarget(null)}
          onDone={() => {
            setConfigTarget(null);
            void loadContainers();
          }}
        />
      )}
    </div>
  );
}
