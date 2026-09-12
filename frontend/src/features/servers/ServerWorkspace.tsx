import { useTabsStore } from '../../store/tabs';
import { OverviewPanel } from './OverviewPanel';
import type { session } from '../../../wailsjs/go/models';

const MODULES = [
  { key: 'overview', label: 'Overview' },
  { key: 'files', label: 'Files' },
  { key: 'terminal', label: 'Terminal' },
  { key: 'services', label: 'Services' },
  { key: 'docker', label: 'Docker' },
];

// Isi satu tab: nav modul di kiri + panel konten modul aktif di kanan.
// Modul di sini masih placeholder — files/terminal/docker/dll akan
// di-porting bertahap dari homepoin sebagai vertical slice terpisah
// (internal/modules/<nama>), lalu didaftarkan di sini seperti MODULES di
// atas. Yang sudah nyata & bisa dites sekarang: berpindah modul DI DALAM
// satu tab hanya mengubah `activeModule`, tidak reload/reconnect apapun.
export function ServerWorkspace({ tab }: { tab: session.Tab }) {
  const setModule = useTabsStore((s) => s.setModule);

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
        <h2>{MODULES.find((m) => m.key === tab.activeModule)?.label ?? tab.activeModule}</h2>
        {tab.activeModule === 'overview' ? (
          <OverviewPanel serverId={tab.serverId} />
        ) : (
          <p className="workspace__placeholder">
            Modul <code>{tab.activeModule}</code> untuk server <code>{tab.serverId}</code> akan
            tampil di sini setelah di-porting dari homepoin.
          </p>
        )}
      </div>
    </div>
  );
}
