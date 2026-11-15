import { useEffect, useRef, useState } from 'react';
import { X, PackagePlus, TriangleAlert, Terminal } from 'lucide-react';
import { StreamArchiveToolInstall, StopDockerStream } from '../../../wailsjs/go/main/App';
import { files } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { pushStreamLine, renderStreamLines } from '../../components/streamLines';

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Batas baris log yang disimpan — `apt-get update` saja bisa ratusan baris.
const MAX_LINES = 2000;

// Muncul saat tombol Ekstrak ditekan untuk arsip yang alat ekstraksinya
// belum ada di server (`unzip` absen di hampir semua image minimal).
//
// Backend yang memutuskan paket apa yang dipasang — yang dikirim dari sini
// cuma format arsipnya. Lihat internal/modules/files/archivetool.go.
export function ArchiveToolModal({
  serverId,
  info,
  archiveName,
  onReady,
  onClose,
}: {
  serverId: string;
  info: files.ArchiveToolInfo;
  archiveName: string;
  // Dipanggil sesudah pemasangan berhasil — pemanggil melanjutkan ekstraksi
  // yang tadi tertunda, supaya satu klik tetap terasa seperti satu tindakan.
  onReady: () => void;
  onClose: () => void;
}) {
  const [lines, setLines] = useState<string[]>([]);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const streamIdRef = useRef<string | null>(null);
  const unsubRef = useRef<(() => void) | null>(null);
  const logRef = useRef<HTMLPreElement>(null);

  // Modal ditutup di tengah pemasangan: hentikan stream DAN lepas
  // listener-nya — tanpa ini listener tetap hidup dan memanggil setState
  // pada komponen yang sudah tidak ada.
  useEffect(() => {
    return () => {
      unsubRef.current?.();
      if (streamIdRef.current) void StopDockerStream(streamIdRef.current);
    };
  }, []);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight });
  }, [lines]);

  function finish() {
    unsubRef.current?.();
    unsubRef.current = null;
    streamIdRef.current = null;
    setRunning(false);
  }

  async function handleInstall() {
    setLines([]);
    setError(null);
    setRunning(true);
    try {
      const streamId = await StreamArchiveToolInstall(serverId, info.format);
      streamIdRef.current = streamId;
      unsubRef.current = EventsOn(`files:install:${streamId}`, (evt: StreamLineEvent) => {
        if (evt.type === 'line' && evt.line !== undefined) {
          setLines((l) => {
            const next = pushStreamLine(l, evt.line as string);
            return next.length > MAX_LINES ? next.slice(-MAX_LINES) : next;
          });
        } else if (evt.type === 'error') {
          setError(evt.message ?? 'Pemasangan gagal.');
          finish();
        } else if (evt.type === 'end') {
          finish();
          setDone(true);
        }
      });
    } catch (e) {
      setError(String(e));
      finish();
    }
  }

  const paket = info.packages.join(', ');
  const kurang = info.missing.join(', ');

  return (
    <div
      className="modal-overlay"
      onMouseDown={(e) => e.target === e.currentTarget && !running && onClose()}
    >
      <div className="modal-card">
        <div className="modal-card__header">
          <h2>Alat ekstraksi belum ada</h2>
          <button className="modal-card__close" disabled={running} onClick={onClose}>
            <X size={18} />
          </button>
        </div>

        <div className="modal-card__body">
          <p>
            <TriangleAlert size={13} /> Untuk membuka <strong>{archiveName}</strong>, server butuh{' '}
            <code>{kurang}</code> — dan perintah itu tidak ada di sana.
          </p>
          <p className="archive-tool__distro">
            {info.distroName} · paket: {info.packageManager || 'tidak diketahui'}
          </p>

          {info.blocker && (
            <>
              <p className="overview__error">{info.blocker}</p>
              {info.manualCommand && (
                <p className="archive-tool__manual">
                  <Terminal size={13} /> Jalankan sendiri di server:{' '}
                  <code>{info.manualCommand}</code>
                </p>
              )}
            </>
          )}

          {info.canInstall && !done && (
            <p className="archive-tool__note">
              poinhost bisa memasangnya lewat <code>{info.packageManager}</code>. Yang dipasang
              hanya <code>{paket}</code>.
            </p>
          )}

          {done && <p className="archive-tool__ok">{paket} terpasang. Melanjutkan ekstraksi...</p>}
          {error && <p className="overview__error">{error}</p>}

          {lines.length > 0 && (
            <pre className="docker-engine__log" ref={logRef}>
              {renderStreamLines(lines)}
            </pre>
          )}
        </div>

        <div className="modal-card__footer">
          <button className="btn btn--ghost" disabled={running} onClick={onClose}>
            {done ? 'Tutup' : 'Batal'}
          </button>
          {info.canInstall && !done && (
            <button className="btn btn--primary" disabled={running} onClick={() => void handleInstall()}>
              {running ? <span className="spinner" /> : <PackagePlus size={13} />}{' '}
              {running ? 'Memasang...' : `Pasang ${paket}`}
            </button>
          )}
          {done && (
            <button className="btn btn--primary" onClick={onReady}>
              Ekstrak sekarang
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
