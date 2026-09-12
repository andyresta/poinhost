import { useEffect, useRef, useState } from 'react';
import { StartDockerEngine, StreamDockerEngineInstall, StopDockerStream } from '../../../wailsjs/go/main/App';
import { docker } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Ditampilkan menggantikan tabel container kalau Docker belum terpasang atau
// servicenya sedang tidak aktif di server. "Install" menjalankan skrip resmi
// get.docker.com lewat PTY (lihat internal/modules/docker/install.go),
// progresnya mengalir baris-per-baris lewat event "docker:install:<id>" —
// sama seperti terminal, cuma read-only (tidak menerima input balik).
export function EngineInstallWizard({
  serverId,
  status,
  onChange,
}: {
  serverId: string;
  status: docker.EngineStatus;
  onChange: () => void;
}) {
  const [lines, setLines] = useState<string[]>([]);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const streamIdRef = useRef<string | null>(null);
  const logRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight });
  }, [lines]);

  useEffect(() => {
    return () => {
      if (streamIdRef.current) void StopDockerStream(streamIdRef.current);
    };
  }, []);

  async function handleStart() {
    setRunning(true);
    setError(null);
    try {
      await StartDockerEngine(serverId);
      onChange();
    } catch (e) {
      setError(String(e));
    } finally {
      setRunning(false);
    }
  }

  async function handleInstall() {
    setLines([]);
    setError(null);
    setRunning(true);
    const streamId = await StreamDockerEngineInstall(serverId);
    streamIdRef.current = streamId;
    const unsub = EventsOn(`docker:install:${streamId}`, (evt: StreamLineEvent) => {
      if (evt.type === 'line' && evt.line) {
        setLines((l) => [...l, evt.line as string]);
      } else if (evt.type === 'error') {
        setError(evt.message ?? 'Instalasi gagal');
        setRunning(false);
        streamIdRef.current = null;
        unsub();
      } else if (evt.type === 'end') {
        setRunning(false);
        streamIdRef.current = null;
        unsub();
        onChange();
      }
    });
  }

  if (status.installed && !status.active) {
    return (
      <div className="docker-engine">
        <p>
          Docker sudah terpasang (versi {status.version || '?'}) tapi service-nya sedang tidak aktif di
          server ini.
        </p>
        {error && <p className="overview__error">{error}</p>}
        <button className="btn btn--primary" disabled={running} onClick={() => void handleStart()}>
          {running ? 'Menjalankan…' : 'Jalankan Docker'}
        </button>
      </div>
    );
  }

  return (
    <div className="docker-engine">
      <p>
        Docker belum terpasang di server ini
        {status.distroName ? ` (distro terdeteksi: ${status.distroName})` : ''}.
      </p>
      {!status.canInstall && (
        <p className="overview__error">
          Distro/package manager server ini belum didukung untuk instalasi otomatis — instal Docker
          secara manual lalu refresh halaman ini.
        </p>
      )}
      {status.canInstall && (
        <button className="btn btn--primary" disabled={running} onClick={() => void handleInstall()}>
          {running ? 'Menginstal…' : 'Install Docker'}
        </button>
      )}
      {error && <p className="overview__error">{error}</p>}
      {lines.length > 0 && (
        <pre className="docker-engine__log" ref={logRef}>
          {lines.join('\n')}
        </pre>
      )}
    </div>
  );
}
