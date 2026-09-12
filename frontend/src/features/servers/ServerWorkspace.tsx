import { useEffect, useState } from 'react';
import { useTabsStore } from '../../store/tabs';
import { OverviewPanel } from './OverviewPanel';
import { TerminalPanel } from './TerminalPanel';
import { FilesPanel } from './FilesPanel';
import type { session } from '../../../wailsjs/go/models';

const MODULES = [
  { key: 'overview', label: 'Overview' },
  { key: 'files', label: 'Files' },
  { key: 'terminal', label: 'Terminal' },
  { key: 'services', label: 'Services' },
  { key: 'docker', label: 'Docker' },
];

// Modul yang punya koneksi/state nyata yang MAHAL untuk dibuang & dibuat
// ulang (sesi shell PTY, direktori & seleksi file yang sedang dibuka, dst)
// — begitu pertama kali dikunjungi, panelnya tetap MOUNTED (di-toggle
// `hidden`, bukan unmount) selama tab ini masih terbuka. Modul lain masih
// placeholder, belum ada state yang perlu dipertahankan, jadi render
// sederhana saja.
const STATEFUL_MODULES = new Set(['overview', 'terminal', 'files']);

// Isi satu tab: nav modul di kiri + panel konten modul aktif di kanan.
// Modul files/services/docker di sini masih placeholder — akan di-porting
// bertahap dari homepoin sebagai vertical slice terpisah, lalu didaftarkan
// di MODULES di atas. Berpindah modul DI DALAM satu tab hanya mengubah
// `activeModule` (dan visibilitas panel via `hidden`) — tidak pernah
// reload/reconnect apa pun, termasuk sesi terminal yang sedang berjalan.
export function ServerWorkspace({ tab }: { tab: session.Tab }) {
  const setModule = useTabsStore((s) => s.setModule);
  const [visited, setVisited] = useState<Set<string>>(() => new Set([tab.activeModule]));

  useEffect(() => {
    if (STATEFUL_MODULES.has(tab.activeModule) && !visited.has(tab.activeModule)) {
      setVisited((v) => new Set(v).add(tab.activeModule));
    }
  }, [tab.activeModule, visited]);

  const activeLabel = MODULES.find((m) => m.key === tab.activeModule)?.label ?? tab.activeModule;
  const isPlaceholder = !STATEFUL_MODULES.has(tab.activeModule);

  return (
    <div className="workspace">
      <nav className="workspace__nav">
        {MODULES.map((m) => (
          <button
            key={m.key}
            className={`workspace__nav-item${tab.activeModule === m.key ? ' workspace__nav-item--active' : ''}`}
            onClick={() => void setModule(tab.id, m.key)}
          >
            {m.label}
          </button>
        ))}
      </nav>
      <div className="workspace__panel">
        <h2>{activeLabel}</h2>

        {visited.has('overview') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'overview'}>
            <OverviewPanel serverId={tab.serverId} />
          </div>
        )}

        {visited.has('terminal') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'terminal'}>
            <TerminalPanel tabId={tab.id} active={tab.activeModule === 'terminal'} />
          </div>
        )}

        {visited.has('files') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'files'}>
            <FilesPanel serverId={tab.serverId} />
          </div>
        )}

        {isPlaceholder && (
          <p className="workspace__placeholder">
            Modul <code>{tab.activeModule}</code> untuk server <code>{tab.serverId}</code> akan
            tampil di sini setelah di-porting dari homepoin.
          </p>
        )}
      </div>
    </div>
  );
}
