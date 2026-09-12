import { useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { OpenTerminal, WriteTerminal, ResizeTerminal, CloseTerminal } from '../../../wailsjs/go/main/App';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

function decodeBase64(b64: string): Uint8Array {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

// Terminal PTY interaktif untuk SATU tab, lewat xterm.js. Komponen ini
// dipasang SEKALI per tab dan tetap mounted (di-toggle `hidden`, bukan
// unmount) selama tab-nya masih terbuka — lihat ServerWorkspace.tsx — jadi
// sesi shell + scrollback-nya tidak hilang saat user pindah ke modul lain
// lalu balik lagi. Sesi baru benar-benar ditutup saat TAB-nya ditutup
// (lewat App.CloseServerTab -> terminalSvc.CloseAllForTab di backend).
export function TerminalPanel({ tabId, active }: { tabId: string; active: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const sessionIdRef = useRef<string | null>(null);

  useEffect(() => {
    let disposed = false;
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
    termRef.current = term;
    fitAddonRef.current = fitAddon;

    if (containerRef.current) {
      term.open(containerRef.current);
      fitAddon.fit();
    }

    const sendResize = (sessionId: string) => {
      fitAddon.fit();
      void ResizeTerminal(sessionId, term.cols, term.rows);
    };

    void (async () => {
      let sessionId: string;
      try {
        sessionId = await OpenTerminal(tabId);
      } catch (e) {
        term.write(`\r\n\x1b[31mGagal membuka terminal: ${String(e)}\x1b[0m\r\n`);
        return;
      }
      if (disposed) {
        void CloseTerminal(sessionId);
        return;
      }
      sessionIdRef.current = sessionId;

      unsubData = EventsOn(`terminal:output:${sessionId}`, (b64: string) => {
        term.write(decodeBase64(b64));
      });
      unsubExit = EventsOn(`terminal:exit:${sessionId}`, () => {
        term.write('\r\n\x1b[31m[sesi terminal terputus]\x1b[0m\r\n');
      });

      term.onData((data) => {
        void WriteTerminal(sessionId, data);
      });

      sendResize(sessionId);
      resizeObserver = new ResizeObserver(() => sendResize(sessionId));
      if (containerRef.current) resizeObserver.observe(containerRef.current);
    })();

    return () => {
      disposed = true;
      resizeObserver?.disconnect();
      unsubData?.();
      unsubExit?.();
      if (sessionIdRef.current) void CloseTerminal(sessionIdRef.current);
      term.dispose();
    };
    // Sengaja cuma jalan sekali per tabId (bukan tiap `active` berubah) —
    // lihat efek terpisah di bawah untuk penanganan show/hide.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabId]);

  // Saat panel ini disembunyikan (`hidden`, bukan unmount), elemennya punya
  // ukuran 0 sehingga ResizeObserver tidak berguna. Begitu ditampilkan lagi,
  // paksa fit() + kirim ukuran terbaru — kalau tidak, xterm bisa salah
  // render di ukuran lama sesaat sebelum ada resize window berikutnya.
  useEffect(() => {
    if (active && fitAddonRef.current && sessionIdRef.current) {
      fitAddonRef.current.fit();
      const term = termRef.current;
      if (term) void ResizeTerminal(sessionIdRef.current, term.cols, term.rows);
    }
  }, [active]);

  return <div ref={containerRef} className="terminal-panel" />;
}
