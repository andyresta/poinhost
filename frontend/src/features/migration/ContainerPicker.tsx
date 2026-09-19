import { useCallback, useEffect, useState } from 'react';
import { Box, RefreshCw } from 'lucide-react';
import { ListDockerContainers } from '../../../wailsjs/go/main/App';
import type { docker, servers } from '../../../wailsjs/go/models';
import { useT } from '../../i18n';

// Pemilih container Docker satu server — padanan RemoteBrowser tapi untuk
// migrasi Docker: bukan menjelajah folder, cuma memilih SATU container dari
// daftar `docker ps` server itu (dipakai sisi sumber). Memakai binding
// ListDockerContainers yang sudah ada dari panel Docker biasa, bukan
// binding baru — daftar container tidak berubah tergantung dipakai dari
// menu mana.
export function ContainerPicker({
  serverList,
  serverId,
  onServerChange,
  containerId,
  onContainerChange,
  disabled,
}: {
  serverList: servers.Server[];
  serverId: string;
  onServerChange: (id: string) => void;
  containerId: string;
  onContainerChange: (id: string, name: string) => void;
  disabled?: boolean;
}) {
  const t = useT();
  const [containers, setContainers] = useState<docker.ContainerInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!serverId) {
      setContainers([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await ListDockerContainers(serverId);
      setContainers(res.containers ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [serverId]);

  // Ganti server berarti daftar containernya lain sama sekali — pilihan
  // sebelumnya (dari server lain) tidak berarti apa-apa lagi di sini.
  useEffect(() => {
    void load();
    onContainerChange('', '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  return (
    <div className="mig-browser">
      <div className="mig-browser__head">
        <select
          className="mig-browser__server"
          value={serverId}
          disabled={disabled}
          onChange={(e) => onServerChange(e.target.value)}
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
          disabled={!serverId || loading || disabled}
          onClick={() => void load()}
        >
          <RefreshCw size={13} />
        </button>
      </div>

      <div className="mig-browser__list">
        <div className="mig-browser__scroll">
          {!serverId && <div className="mig-browser__empty">{t('migration.pickServerFirst')}</div>}
          {serverId && error && <div className="mig-browser__error">{error}</div>}
          {serverId && !error && (
            <table className="mig-browser__table">
              <tbody>
                {containers.map((c) => (
                  <tr
                    key={c.id}
                    className={containerId === c.id ? 'mig-browser__row--selected' : undefined}
                    onClick={() => !disabled && onContainerChange(c.id, c.name)}
                  >
                    <td className="mig-browser__col-check">
                      <input
                        type="radio"
                        checked={containerId === c.id}
                        disabled={disabled}
                        readOnly
                      />
                    </td>
                    <td className="mig-browser__name">
                      <span className="mig-browser__icon">
                        <Box size={13} />
                      </span>
                      <span className="mig-browser__ellipsis">{c.name}</span>
                    </td>
                    <td className="mig-browser__size">{c.state}</td>
                  </tr>
                ))}
                {containers.length === 0 && (
                  <tr>
                    <td colSpan={3} className="mig-browser__empty">
                      {t('migration.docker.noContainers')}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          )}
        </div>
        {loading && <div className="mig-browser__loading">{t('common.loading')}</div>}
      </div>
    </div>
  );
}
