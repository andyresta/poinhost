import { useEffect, useRef, useState } from 'react';
import { DockerContainerLogs, StreamDockerContainerLogs, StopDockerStream } from '../../../wailsjs/go/main/App';
import { docker } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface LogStreamEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Menampilkan log container: snapshot tail dulu (biar langsung ada isi),
// lalu tersambung ke stream `docker logs -f` realtime lewat event
// "docker:logs:<streamId>" — streamnya jalan di koneksi dedicated
// (executor.ExecStreamDedicated), bukan slot shared, jadi tidak ikut
// mengantre di belakang operasi Docker lain ke server yang sama.
export function ContainerLogsModal({
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
  const [lines, setLines] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [live, setLive] = useState(false);
  const streamIdRef = useRef<string | null>(null);
  const logRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight });
  }, [lines]);

  useEffect(() => {
    let cancelled = false;
    let unsub: (() => void) | null = null;

    void (async () => {
      try {
        const snapshot = await DockerContainerLogs(new docker.LogsRequest({ serverId, containerId, lines: 300 }));
        if (cancelled) return;
        setLines(snapshot.content ? snapshot.content.split('\n') : []);
      } catch (e) {
        if (!cancelled) setError(String(e));
        return;
      }
      if (cancelled) return;

      const streamId = await StreamDockerContainerLogs(new docker.LogsRequest({ serverId, containerId, lines: 0 }));
      if (cancelled) {
        void StopDockerStream(streamId);
        return;
      }
      streamIdRef.current = streamId;
      setLive(true);
      unsub = EventsOn(`docker:logs:${streamId}`, (evt: LogStreamEvent) => {
        if (evt.type === 'line' && evt.line !== undefined) {
          setLines((l) => [...l, evt.line as string]);
        } else if (evt.type === 'error') {
          setError(evt.message ?? 'Stream log terputus');
          setLive(false);
        } else if (evt.type === 'end') {
          setLive(false);
        }
      });
    })();

    return () => {
      cancelled = true;
      unsub?.();
      if (streamIdRef.current) void StopDockerStream(streamIdRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, containerId]);

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--editor">
        <div className="modal-card__header">
          <h2>
            Log — {name}
            {live && <span className="docker-live-dot" title="Live" />}
          </h2>
          <button className="modal-card__close" onClick={onClose}>
            ×
          </button>
        </div>
        <div className="modal-card__body modal-card__body--editor">
          {error && <p className="overview__error">{error}</p>}
          <pre className="docker-engine__log" ref={logRef}>
            {lines.join('\n')}
          </pre>
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
