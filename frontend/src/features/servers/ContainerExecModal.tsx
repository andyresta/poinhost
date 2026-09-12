import { useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { OpenDockerExec, WriteTerminal, ResizeTerminal, CloseTerminal } from '../../../wailsjs/go/main/App';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

function decodeBase64(b64: string): Uint8Array {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

// "Exec ke dalam container" (docker exec -it <id> bash) sebagai modal PTY —
// dibangun di atas terminal.Service yang SAMA dengan Terminal VPS biasa
// (lihat OpenDockerExec di app.go: cuma beda perintah awal yang dijalankan,
// bukan shell login), jadi dapat gratis semua infrastruktur PTY/xterm.js
// yang sudah ada tanpa jalur streaming terpisah. Beda dari TerminalPanel,
// modal ini SENGAJA tidak di-keep-alive — sesi ditutup begitu modal ditutup,
// karena "masuk ke container tertentu" adalah tindakan sesaat, bukan sesi
// kerja yang mau dipertahankan lintas perpindahan modul seperti Terminal VPS.
export function ContainerExecModal({
  tabId,
  containerId,
  name,
  onClose,
}: {
  tabId: string;
  containerId: string;
  name: string;
  onClose: () => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [shell, setShell] = useState<'auto' | 'bash' | 'sh'>('auto');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let disposed = false;
    let sessionId: string | null = null;
    let unsubData: (() => void) | null = null;
    let unsubExit: (() => void) | null = null;
    let resizeObserver: ResizeObserver | null = null;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Consolas, monospace',
      theme: { background: '#12192a', foreground: '#e5e7eb' },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    if (containerRef.current) {
      term.open(containerRef.current);
      fitAddon.fit();
    }

    void (async () => {
      try {
        sessionId = await OpenDockerExec(tabId, containerId, shell);
      } catch (e) {
        setError(String(e));
        return;
      }
      if (disposed) {
        void CloseTerminal(sessionId);
        return;
      }

      unsubData = EventsOn(`terminal:output:${sessionId}`, (b64: string) => {
        term.write(decodeBase64(b64));
      });
      unsubExit = EventsOn(`terminal:exit:${sessionId}`, () => {
        term.write('\r\n\x1b[31m[sesi exec berakhir]\x1b[0m\r\n');
      });

      term.onData((data) => {
        if (sessionId) void WriteTerminal(sessionId, data);
      });

      fitAddon.fit();
      void ResizeTerminal(sessionId, term.cols, term.rows);
      resizeObserver = new ResizeObserver(() => {
        fitAddon.fit();
        if (sessionId) void ResizeTerminal(sessionId, term.cols, term.rows);
      });
      if (containerRef.current) resizeObserver.observe(containerRef.current);
    })();

    return () => {
      disposed = true;
      resizeObserver?.disconnect();
      unsubData?.();
      unsubExit?.();
      if (sessionId) void CloseTerminal(sessionId);
      term.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabId, containerId, shell]);

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--editor">
        <div className="modal-card__header">
          <h2>Exec — {name}</h2>
          <label className="docker-exec__shell">
            Shell
            <select value={shell} onChange={(e) => setShell(e.target.value as 'auto' | 'bash' | 'sh')}>
              <option value="auto">auto</option>
              <option value="bash">bash</option>
              <option value="sh">sh</option>
            </select>
          </label>
          <button className="modal-card__close" onClick={onClose}>
            ×
          </button>
        </div>
        <div className="modal-card__body modal-card__body--editor">
          {error && <p className="overview__error">{error}</p>}
          <div ref={containerRef} className="terminal-panel" />
        </div>
      </div>
    </div>
  );
}
