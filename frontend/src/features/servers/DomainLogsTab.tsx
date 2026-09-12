import { useEffect, useRef, useState } from 'react';
import { GetWebsiteLog, StreamWebsiteLog, StopDockerStream } from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface LogStreamEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Tab Logs — access.log/error.log domain: snapshot dulu, lalu tersambung ke
// stream `tail -f` realtime (event Wails, pola sama dengan log container
// Docker) selama tab ini terlihat.
export function DomainLogsTab({ serverId, domain }: { serverId: string; domain: string }) {
  const [logType, setLogType] = useState<'access' | 'error'>('access');
  const [lines, setLines] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [live, setLive] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const streamIdRef = useRef<string | null>(null);
  const logRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight });
  }, [lines]);

  useEffect(() => {
    let cancelled = false;
    let unsub: (() => void) | null = null;
    setLines([]);
    setError(null);
    setNotFound(false);
    setLive(false);

    void (async () => {
      try {
        const snapshot = await GetWebsiteLog(new website.LogReadRequest({ serverId, domain, logType, lines: 300 }));
        if (cancelled) return;
        if (!snapshot.exists) {
          setNotFound(true);
          return;
        }
        setLines(snapshot.content ? snapshot.content.split('\n') : []);
      } catch (e) {
        if (!cancelled) setError(String(e));
        return;
      }
      if (cancelled) return;

      const streamId = await StreamWebsiteLog(new website.LogReadRequest({ serverId, domain, logType, lines: 0 }));
      if (cancelled) {
        void StopDockerStream(streamId);
        return;
      }
      streamIdRef.current = streamId;
      setLive(true);
      unsub = EventsOn(`website:logs:${streamId}`, (evt: LogStreamEvent) => {
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
  }, [serverId, domain, logType]);

  return (
    <div>
      <div className="files-panel__toolbar">
        <div className="segmented">
          <button className={`segmented__item${logType === 'access' ? ' segmented__item--active' : ''}`} onClick={() => setLogType('access')}>
            Access
          </button>
          <button className={`segmented__item${logType === 'error' ? ' segmented__item--active' : ''}`} onClick={() => setLogType('error')}>
            Error
          </button>
        </div>
        {live && <span className="docker-live-dot" title="Live" />}
      </div>
      {error && <p className="overview__error">{error}</p>}
      {notFound && <p className="workspace__placeholder">File log belum ada — domain ini mungkin belum pernah diakses.</p>}
      {!notFound && (
        <pre className="docker-engine__log" ref={logRef} style={{ maxHeight: 420 }}>
          {lines.join('\n')}
        </pre>
      )}
    </div>
  );
}
