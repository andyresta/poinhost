import { useEffect, useMemo, useState } from 'react';
import { RotateCw, Play, Square, RefreshCw, Power, PowerOff, TriangleAlert, ScrollText } from 'lucide-react';
import { ListSystemServices, SystemServiceAction } from '../../../wailsjs/go/main/App';
import type { services } from '../../../wailsjs/go/models';
import { useConfirm } from '../../components/ConfirmDialog';
import { useT } from '../../i18n';
import { ServiceLogsTab } from './ServiceLogsTab';

type Filter = 'all' | 'running' | 'enabled' | 'stopped';
type SubTab = 'services' | 'logs';

// Modul Services: kelola unit systemd di server — jalan/berhenti sekarang
// DAN autostart saat boot.
//
// Dua hal itu sengaja ditampilkan sebagai dua kolom terpisah karena memang
// independen di systemd: service bisa "running tapi tidak autostart" (hilang
// setelah reboot) atau "enabled tapi sedang mati". Menggabungkannya jadi
// satu tombol on/off akan menyembunyikan justru perbedaan yang paling sering
// bikin salah paham.
export function ServicesPanel({ serverId }: { serverId: string }) {
  const confirm = useConfirm();
  const t = useT();
  // State disimpan terurai (bukan objek ListResponse utuh): kelas hasil
  // generate Wails membawa method convertValues, dan meng-spread-nya untuk
  // update satu baris akan membuang method itu.
  const [items, setItems] = useState<services.ServiceInfo[]>([]);
  const [systemdAvailable, setSystemdAvailable] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busyName, setBusyName] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<Filter>('all');
  const [subTab, setSubTab] = useState<SubTab>('services');
  // Unit yang dipilih saat user menekan tombol "Logs" di satu baris service.
  // String kosong = seluruh sistem.
  const [logUnit, setLogUnit] = useState('');

  async function load() {
    setError(null);
    setRefreshing(true);
    try {
      const res = await ListSystemServices(serverId);
      setItems(res.services ?? []);
      setSystemdAvailable(res.systemdAvailable);
      setLoaded(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    setLoading(true);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  // Hasil aksi mengembalikan status TERBARU service itu saja — dipakai
  // memperbarui satu baris di tempat, bukan menarik ulang seluruh daftar
  // (di server dengan ratusan unit, refresh penuh terasa berat untuk satu
  // klik tombol).
  async function runAction(name: string, action: 'start' | 'stop' | 'restart' | 'enable' | 'disable') {
    setBusyName(name);
    setError(null);
    try {
      const updated = await SystemServiceAction(serverId, name, action);
      setItems((prev) => prev.map((s) => (s.name === name ? updated : s)));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusyName(null);
    }
  }

  async function handleStop(svc: services.ServiceInfo) {
    const ok = await confirm({
      title: t('svc.stop'),
      message: t('svc.stopConfirm', { name: svc.name }),
      detail: t('svc.stopDetail'),
      confirmLabel: t('svc.stop'),
      danger: true,
    });
    if (ok) await runAction(svc.name, 'stop');
  }

  async function handleDisable(svc: services.ServiceInfo) {
    const ok = await confirm({
      title: t('svc.disable'),
      message: t('svc.disableConfirm', { name: svc.name }),
      detail: t('svc.disableDetail'),
      confirmLabel: t('svc.disable'),
      danger: true,
    });
    if (ok) await runAction(svc.name, 'disable');
  }

  const filtered = useMemo(() => {
    const all = items;
    const q = query.trim().toLowerCase();
    return all.filter((s) => {
      if (q && !s.name.toLowerCase().includes(q) && !(s.description ?? '').toLowerCase().includes(q)) return false;
      if (filter === 'running') return s.running;
      if (filter === 'stopped') return !s.running;
      if (filter === 'enabled') return s.enabled;
      return true;
    });
  }, [items, query, filter]);

  // Tanpa systemd kedua sub-tab sama-sama tidak berlaku (journald ikut
  // systemd), jadi pesannya menggantikan seluruh panel — bukan per tab.
  if (loaded && !systemdAvailable) {
    return (
      <div className="docker-engine">
        <p>
          <TriangleAlert size={13} /> {t('svc.noSystemd')}
        </p>
      </div>
    );
  }

  const counts = {
    all: items.length,
    running: items.filter((s) => s.running).length,
    enabled: items.filter((s) => s.enabled).length,
    stopped: items.filter((s) => !s.running).length,
  };

  return (
    <div className="docker-panel">
      <nav className="docker-panel__subnav">
        {(['services', 'logs'] as SubTab[]).map((key) => (
          <button
            key={key}
            className={`docker-panel__subnav-item${subTab === key ? ' docker-panel__subnav-item--active' : ''}`}
            onClick={() => setSubTab(key)}
          >
            {t(`svc.tab.${key}` as 'svc.tab.services')}
          </button>
        ))}
      </nav>

      {subTab === 'logs' && (
        <ServiceLogsTab serverId={serverId} units={items.map((s) => s.name)} initialUnit={logUnit} />
      )}

      {subTab === 'services' &&
        (loading ? (
          <p className="workspace__placeholder">{t('common.loading')}</p>
        ) : (
          <div className="files-panel">
            <div className="files-panel__toolbar">
              <button className="btn btn--sm" disabled={refreshing} onClick={() => void load()}>
                {refreshing ? <span className="spinner" /> : <RotateCw size={13} />} {t('common.refresh')}
              </button>

              <div className="segmented" style={{ marginBottom: 0 }}>
                {(['all', 'running', 'enabled', 'stopped'] as Filter[]).map((f) => (
                  <button
                    key={f}
                    className={`segmented__item${filter === f ? ' segmented__item--active' : ''}`}
                    onClick={() => setFilter(f)}
                  >
                    {t(`svc.filter.${f}` as 'svc.filter.all')} ({counts[f]})
                  </button>
                ))}
              </div>

              <input
                placeholder={t('svc.search')}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                style={{ marginLeft: 'auto', maxWidth: 220 }}
              />
            </div>

            {error && <p className="overview__error">{error}</p>}

            <div className="files-panel__table-wrap">
              <div className="files-panel__table-scroll">
              <table className="files-panel__table">
                <thead>
                  <tr>
                    <th>{t('svc.colService')}</th>
                    <th>{t('svc.colState')}</th>
                    <th>{t('svc.colAutostart')}</th>
                    <th className="files-panel__col-actions" />
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((s) => {
                    const busy = busyName === s.name;
                    return (
                      <tr key={s.name}>
                        <td className="files-panel__name">
                          <div style={{ fontWeight: 600 }}>{s.name}</div>
                          {s.description && (
                            <div
                              style={{
                                fontSize: 11,
                                opacity: 0.6,
                                whiteSpace: 'normal',
                              }}
                            >
                              {s.description}
                            </div>
                          )}
                        </td>
                        <td>
                          <span
                            className={`docker-badge ${s.running ? 'docker-badge--running' : 'docker-badge--stopped'}`}
                          >
                            {s.running ? t('svc.running') : t('svc.stopped')}
                          </span>
                          {s.subState && <span style={{ fontSize: 11, opacity: 0.5 }}> {s.subState}</span>}
                        </td>
                        <td>
                          <span
                            className={`docker-badge ${s.enabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}
                          >
                            {s.enabled ? t('svc.enabled') : t('svc.disabled')}
                          </span>
                          {!s.canEnable && s.unitFileState && (
                            <span style={{ fontSize: 11, opacity: 0.5 }} title={t('svc.cannotToggle')}>
                              {' '}
                              {s.unitFileState}
                            </span>
                          )}
                        </td>
                        <td className="files-panel__row-actions">
                          {busy && <span className="spinner" />}
                          {s.running ? (
                            <button title={t('svc.stop')} disabled={busy} onClick={() => void handleStop(s)}>
                              <Square size={14} />
                            </button>
                          ) : (
                            <button
                              title={t('svc.start')}
                              disabled={busy}
                              onClick={() => void runAction(s.name, 'start')}
                            >
                              <Play size={14} />
                            </button>
                          )}
                          <button
                            title={t('svc.restart')}
                            disabled={busy || !s.running}
                            onClick={() => void runAction(s.name, 'restart')}
                          >
                            <RefreshCw size={14} />
                          </button>
                          <button
                            title={t('journal.viewFor', { name: s.name })}
                            onClick={() => {
                              setLogUnit(s.name);
                              setSubTab('logs');
                            }}
                          >
                            <ScrollText size={14} />
                          </button>
                          {s.enabled ? (
                            <button
                              title={s.canEnable ? t('svc.disable') : t('svc.cannotToggle')}
                              disabled={busy || !s.canEnable}
                              onClick={() => void handleDisable(s)}
                            >
                              <PowerOff size={14} />
                            </button>
                          ) : (
                            <button
                              title={s.canEnable ? t('svc.enable') : t('svc.cannotToggle')}
                              disabled={busy || !s.canEnable}
                              onClick={() => void runAction(s.name, 'enable')}
                            >
                              <Power size={14} />
                            </button>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                  {filtered.length === 0 && (
                    <tr>
                      <td colSpan={4} className="files-panel__empty">
                        {t('svc.noServices')}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
              </div>
            </div>
          </div>
        ))}
    </div>
  );
}
