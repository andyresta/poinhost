import { useCallback, useEffect, useState } from 'react';
import { Folder, FileText, ArrowUp, RefreshCw } from 'lucide-react';
import { XferListDirectory } from '../../../wailsjs/go/main/App';
import type { filexfer, servers } from '../../../wailsjs/go/models';
import { useT } from '../../i18n';

// Penjelajah folder remote yang dipakai kedua sisi panel migrasi. Sisi
// sumber memakainya untuk MEMILIH item (checkbox), sisi tujuan hanya untuk
// MENENTUKAN folder yang sedang dibuka — dua kebutuhan yang cukup mirip
// untuk berbagi satu komponen, dibedakan lewat prop `selectable`.
export function RemoteBrowser({
  serverList,
  serverId,
  onServerChange,
  path,
  onPathChange,
  selectable,
  selected,
  onToggle,
  disabled,
  reloadToken,
}: {
  serverList: servers.Server[];
  serverId: string;
  onServerChange: (id: string) => void;
  path: string;
  onPathChange: (p: string) => void;
  selectable: boolean;
  selected?: Set<string>;
  onToggle?: (entry: filexfer.DirEntry) => void;
  disabled?: boolean;
  // Dinaikkan pemanggil untuk memaksa muat ulang — dipakai sisi tujuan
  // supaya hasil transfer langsung terlihat tanpa user menekan Refresh.
  reloadToken?: number;
}) {
  const t = useT();
  const [result, setResult] = useState<filexfer.ListDirResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(
    async (target: string) => {
      if (!serverId) {
        setResult(null);
        return;
      }
      setLoading(true);
      setError(null);
      try {
        const res = await XferListDirectory(serverId, target);
        setResult(res);
        onPathChange(res.path);
      } catch (e) {
        setError(String(e));
      } finally {
        setLoading(false);
      }
    },
    [serverId, onPathChange],
  );

  // Ganti server berarti pohon direktorinya lain sama sekali, jadi kembali
  // ke root daripada mencoba membuka path lama yang belum tentu ada.
  useEffect(() => {
    void load('/');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  // Muat ulang di tempat (tetap di folder yang sedang dibuka), bukan balik
  // ke root seperti saat ganti server.
  useEffect(() => {
    if (!reloadToken) return;
    void load(path);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadToken]);

  const crumbs = path.split('/').filter(Boolean);

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
          title={t('migration.up')}
          disabled={!serverId || loading || path === '/'}
          onClick={() => void load(result?.parent || '/')}
        >
          <ArrowUp size={13} />
        </button>
        <button
          className="btn btn--sm"
          title={t('common.refresh')}
          disabled={!serverId || loading}
          onClick={() => void load(path)}
        >
          <RefreshCw size={13} />
        </button>
      </div>

      <div className="mig-browser__crumbs">
        <button className="mig-browser__crumb" onClick={() => void load('/')}>
          /
        </button>
        {crumbs.map((part, i) => (
          <span key={i}>
            <button
              className="mig-browser__crumb"
              onClick={() => void load('/' + crumbs.slice(0, i + 1).join('/'))}
            >
              {part}
            </button>
            <span className="mig-browser__crumb-sep">/</span>
          </span>
        ))}
      </div>

      <div className="mig-browser__list">
        {!serverId && <div className="mig-browser__empty">{t('migration.pickServerFirst')}</div>}
        {serverId && error && <div className="mig-browser__error">{error}</div>}
        {serverId && !error && (
          <table className="mig-browser__table">
            <tbody>
              {(result?.entries ?? []).map((entry) => (
                <tr key={entry.path}>
                  {selectable && (
                    <td className="mig-browser__col-check">
                      <input
                        type="checkbox"
                        checked={selected?.has(entry.path) ?? false}
                        disabled={disabled}
                        onChange={() => onToggle?.(entry)}
                      />
                    </td>
                  )}
                  <td
                    className="mig-browser__name"
                    onDoubleClick={() => entry.isDir && void load(entry.path)}
                  >
                    <span className="mig-browser__icon">
                      {entry.isDir ? <Folder size={13} /> : <FileText size={13} />}
                    </span>
                    <span className="mig-browser__ellipsis">{entry.name}</span>
                  </td>
                  <td className="mig-browser__size">{entry.isDir ? '—' : formatBytes(entry.size)}</td>
                </tr>
              ))}
              {result && result.entries.length === 0 && (
                <tr>
                  <td colSpan={selectable ? 3 : 2} className="mig-browser__empty">
                    {t('migration.emptyFolder')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
        {loading && <div className="mig-browser__loading">{t('common.loading')}</div>}
      </div>
    </div>
  );
}

// formatBytes menampilkan ukuran dalam satuan biner (KiB/MiB) — sama
// seperti yang dilaporkan `du`, supaya angkanya cocok dengan yang dilihat
// user di terminal, bukan versi desimal yang terlihat lebih besar.
export function formatBytes(n: number): string {
  if (!n || n < 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 10 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}
