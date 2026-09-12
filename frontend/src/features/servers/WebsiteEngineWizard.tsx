import { useEffect, useRef, useState } from 'react';
import { StartNginxEngine, StreamWebsiteInstall, StopDockerStream } from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Ditampilkan menggantikan daftar domain kalau Nginx belum terpasang atau
// sedang tidak aktif — sama persis polanya dengan EngineInstallWizard milik
// Docker (StopDockerStream dipakai ulang di sini karena stream-registry di
// app.go memang generik untuk semua jenis stream, bukan cuma Docker).
export function WebsiteEngineWizard({
  serverId,
  status,
  onChange,
}: {
  serverId: string;
  status: website.NginxStatus;
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
      await StartNginxEngine(serverId);
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
    const streamId = await StreamWebsiteInstall(serverId, 'nginx', '');
    streamIdRef.current = streamId;
    const unsub = EventsOn(`website:install:${streamId}`, (evt: StreamLineEvent) => {
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
        <p>Nginx sudah terpasang (versi {status.version || '?'}) tapi sedang tidak aktif di server ini.</p>
        {error && <p className="overview__error">{error}</p>}
        <button className="btn btn--primary" disabled={running} onClick={() => void handleStart()}>
          {running ? 'Menjalankan…' : 'Jalankan Nginx'}
        </button>
      </div>
    );
  }

  return (
    <div className="docker-engine">
      <p>
        Nginx belum terpasang di server ini
        {status.distroName ? ` (distro terdeteksi: ${status.distroName})` : ''}.
      </p>
      {!status.canInstall && (
        <p className="overview__error">
          Distro/package manager server ini belum didukung untuk instalasi otomatis — instal Nginx
          secara manual lalu refresh halaman ini.
        </p>
      )}
      {status.canInstall && (
        <button className="btn btn--primary" disabled={running} onClick={() => void handleInstall()}>
          {running ? 'Menginstal…' : 'Install Nginx'}
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
