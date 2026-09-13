import { useEffect, useRef, useState } from 'react';
import { StreamDockerContainerStats, StopDockerStream } from '../../../wailsjs/go/main/App';
import { docker } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { X } from 'lucide-react';

interface StatsStreamEvent {
  type: 'stats' | 'end' | 'error';
  stats?: docker.StatsResponse;
  message?: string;
}

// Statistik resource container realtime (~1 sampel/detik) lewat `docker
// stats` di koneksi dedicated — sama seperti ContainerLogsModal, dibuka
// selama modal ini terbuka dan dihentikan (StopDockerStream) saat ditutup.
export function ContainerStatsModal({
  serverId,
  containerId,
  name,
  onClose,
}: {
  serverId: string;
  containerId: string;
  name: string;
  onClose: () => void;
}) {
  const [stats, setStats] = useState<docker.StatsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const streamIdRef = useRef<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let unsub: (() => void) | null = null;

    void (async () => {
      const streamId = await StreamDockerContainerStats(serverId, containerId);
      if (cancelled) {
        void StopDockerStream(streamId);
        return;
      }
      streamIdRef.current = streamId;
      unsub = EventsOn(`docker:stats:${streamId}`, (evt: StatsStreamEvent) => {
        if (evt.type === 'stats' && evt.stats) {
          setStats(evt.stats);
        } else if (evt.type === 'error') {
          setError(evt.message ?? 'Stream statistik terputus');
        }
      });
    })();

    return () => {
      cancelled = true;
      unsub?.();
      if (streamIdRef.current) void StopDockerStream(streamIdRef.current);
    };
  }, [serverId, containerId]);

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--small">
        <div className="modal-card__header">
          <h2>Statistik — {name}</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body">
          {error && <p className="overview__error">{error}</p>}
          {!stats && !error && <p className="workspace__placeholder">Menunggu sampel pertama…</p>}
          {stats && (
            <div className="overview__grid">
              <div className="overview__stat">
                <div className="overview__stat-label">CPU</div>
                <div className="overview__stat-value">{stats.cpuPerc}</div>
              </div>
              <div className="overview__stat">
                <div className="overview__stat-label">Memori</div>
                <div className="overview__stat-value">{stats.memUsage}</div>
                <div className="overview__stat-label">{stats.memPerc}</div>
              </div>
              <div className="overview__stat">
                <div className="overview__stat-label">Network I/O</div>
                <div className="overview__stat-value">{stats.netIO}</div>
              </div>
              <div className="overview__stat">
                <div className="overview__stat-label">Block I/O</div>
                <div className="overview__stat-value">{stats.blockIO}</div>
              </div>
              <div className="overview__stat">
                <div className="overview__stat-label">PIDs</div>
                <div className="overview__stat-value">{stats.pids}</div>
              </div>
            </div>
          )}
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
