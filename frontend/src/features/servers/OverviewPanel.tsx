import { useState } from 'react';
import { RotateCw } from 'lucide-react';
import { useTabsStore } from '../../store/tabs';
import { StatusDot } from './StatusDot';

function formatUptime(seconds?: number): string {
  if (!seconds) return '—';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return `${days}h ${hours}j`;
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours}j ${minutes}m`;
}

function pct(used?: number, total?: number): number | null {
  if (!used || !total) return null;
  return Math.round((used / total) * 100);
}

// Panel "Overview" satu server — paritas informasi dengan homepoin
// (OS, hostname, CPU/RAM/disk, uptime), tapi datanya datang dari cache
// Collector yang sudah berjalan di background (lihat store/tabs.ts +
// App.tsx), bukan modul ini yang memicu SSH sendiri. Tombol Refresh cuma
// memanggil RefreshServerStatus untuk melewati jadwal, bukan satu-satunya
// cara data ini bisa terisi.
export function OverviewPanel({ serverId }: { serverId: string }) {
  const status = useTabsStore((s) => s.statuses[serverId]);
  const refreshStatus = useTabsStore((s) => s.refreshStatus);
  const [refreshing, setRefreshing] = useState(false);

  async function handleRefresh() {
    setRefreshing(true);
    try {
      await refreshStatus(serverId);
    } finally {
      setRefreshing(false);
    }
  }

  const memPct = pct(status?.memUsedMb, status?.memTotalMb);
  const diskPct = pct(status?.diskUsedGb, status?.diskTotalGb);

  return (
    <div className="overview">
      <div className="overview__header">
        <div className="overview__status">
          <StatusDot connection={status?.connection} />
          <span>{status?.connection === 'online' ? 'Online' : status?.connection === 'reconnecting' ? 'Menyambung ulang…' : 'Offline'}</span>
        </div>
        <button className="btn btn--ghost btn--sm" onClick={() => void handleRefresh()} disabled={refreshing}>
          {refreshing ? 'Memuat…' : (<><RotateCw size={13} /> Refresh</>)}
        </button>
      </div>

      {status?.metricsError && status.connection !== 'offline' && (
        <p className="overview__error">Gagal ambil metrik terakhir: {status.metricsError}</p>
      )}

      {!status || (!status.hostname && !status.osName) ? (
        <p className="workspace__placeholder">
          Belum ada data metrik{status?.connection === 'offline' ? ' — server sedang offline.' : ', menunggu pengecekan pertama…'}
        </p>
      ) : (
        <div className="overview__grid">
          <Stat label="Hostname" value={status.hostname || '—'} />
          <Stat label="OS" value={status.osName || '—'} />
          <Stat label="Uptime" value={formatUptime(status.uptimeSeconds)} />
          <Stat label="CPU" value={status.cpuUsedPct != null ? `${Math.round(status.cpuUsedPct)}% dari ${status.cpuCores ?? '?'} core` : '—'} bar={status.cpuUsedPct} />
          <Stat label="Memori" value={status.memUsedMb != null ? `${status.memUsedMb} / ${status.memTotalMb} MB` : '—'} bar={memPct ?? undefined} />
          <Stat label="Disk /" value={status.diskUsedGb != null ? `${status.diskUsedGb} / ${status.diskTotalGb} GB` : '—'} bar={diskPct ?? undefined} />
        </div>
      )}

      {status?.metricsCheckedAt && (
        <p className="overview__checked-at">
          Terakhir dicek: {new Date(status.metricsCheckedAt).toLocaleTimeString()}
          {status.metricsStale ? ' (data lama, percobaan terakhir gagal)' : ''}
        </p>
      )}
    </div>
  );
}

function Stat({ label, value, bar }: { label: string; value: string; bar?: number }) {
  return (
    <div className="overview__stat">
      <div className="overview__stat-label">{label}</div>
      <div className="overview__stat-value">{value}</div>
      {bar != null && (
        <div className="overview__bar">
          <div className="overview__bar-fill" style={{ width: `${Math.min(100, bar)}%` }} />
        </div>
      )}
    </div>
  );
}
