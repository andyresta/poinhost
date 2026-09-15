import { useEffect, useState } from 'react';
import { LayoutDashboard, Globe, Database, FolderOpen, TerminalSquare, Settings2, Boxes, Shield, ChevronLeft, ChevronRight } from 'lucide-react';
import { useTabsStore } from '../../store/tabs';
import { useT } from '../../i18n';
import { useWorkspaceNavStore } from '../../store/workspaceNav';
import { OverviewPanel } from './OverviewPanel';
import { TerminalPanel } from './TerminalPanel';
import { FilesPanel } from './FilesPanel';
import { DockerPanel } from './DockerPanel';
import { WebsitePanel } from './WebsitePanel';
import { DatabaseManagerPanel } from './DatabaseManagerPanel';
import { ServicesPanel } from './ServicesPanel';
import { FirewallPanel } from './FirewallPanel';
import type { session } from '../../../wailsjs/go/models';

// Ikon dipakai saat nav diciutkan (hanya ikon yang tampak) — pola yang
// sama dengan sidebar server utama, lihat ServersPage.
const MODULES = [
  { key: 'overview', labelKey: 'module.overview', Icon: LayoutDashboard },
  { key: 'website', labelKey: 'module.website', Icon: Globe },
  { key: 'database', labelKey: 'module.database', Icon: Database },
  { key: 'files', labelKey: 'module.files', Icon: FolderOpen },
  { key: 'terminal', labelKey: 'module.terminal', Icon: TerminalSquare },
  { key: 'services', labelKey: 'module.services', Icon: Settings2 },
  { key: 'firewall', labelKey: 'module.firewall', Icon: Shield },
  { key: 'docker', labelKey: 'module.docker', Icon: Boxes },
] as const;

// Modul yang punya koneksi/state nyata yang MAHAL untuk dibuang & dibuat
// ulang (sesi shell PTY, direktori & seleksi file yang sedang dibuka, dst)
// — begitu pertama kali dikunjungi, panelnya tetap MOUNTED (di-toggle
// `hidden`, bukan unmount) selama tab ini masih terbuka. Modul lain masih
// placeholder, belum ada state yang perlu dipertahankan, jadi render
// sederhana saja.
const STATEFUL_MODULES = new Set(['overview', 'terminal', 'files', 'docker', 'website', 'database', 'services', 'firewall']);

// Isi satu tab: nav modul di kiri + panel konten modul aktif di kanan.
// Modul services di sini masih placeholder — akan di-porting bertahap dari
// homepoin sebagai vertical slice terpisah, lalu didaftarkan di MODULES di
// atas. Berpindah modul DI DALAM satu tab hanya mengubah `activeModule`
// (dan visibilitas panel via `hidden`) — tidak pernah reload/reconnect apa
// pun, termasuk sesi terminal yang sedang berjalan.
export function ServerWorkspace({ tab }: { tab: session.Tab }) {
  const t = useT();
  const navCollapsed = useWorkspaceNavStore((s) => s.collapsed);
  const toggleNav = useWorkspaceNavStore((s) => s.toggle);
  const setModule = useTabsStore((s) => s.setModule);
  const [visited, setVisited] = useState<Set<string>>(() => new Set([tab.activeModule]));

  useEffect(() => {
    if (STATEFUL_MODULES.has(tab.activeModule) && !visited.has(tab.activeModule)) {
      setVisited((v) => new Set(v).add(tab.activeModule));
    }
  }, [tab.activeModule, visited]);

  const activeMeta = MODULES.find((m) => m.key === tab.activeModule);
  const activeLabel = activeMeta ? t(activeMeta.labelKey) : tab.activeModule;
  const isPlaceholder = !STATEFUL_MODULES.has(tab.activeModule);

  return (
    <div className="workspace">
      <nav className={`workspace__nav${navCollapsed ? ' workspace__nav--collapsed' : ''}`}>
        {MODULES.map((m) => (
          <button
            key={m.key}
            className={`workspace__nav-item${tab.activeModule === m.key ? ' workspace__nav-item--active' : ''}`}
            title={t(m.labelKey)}
            onClick={() => void setModule(tab.id, m.key)}
          >
            <m.Icon size={15} />
            {!navCollapsed && <span>{t(m.labelKey)}</span>}
          </button>
        ))}
      </nav>
      <button
        className={`edge-toggle workspace__edge-toggle${navCollapsed ? ' workspace__edge-toggle--collapsed' : ''}`}
        title={navCollapsed ? t('workspace.expandNav') : t('workspace.collapseNav')}
        aria-label={navCollapsed ? t('workspace.expandNav') : t('workspace.collapseNav')}
        onClick={toggleNav}
      >
        {navCollapsed ? <ChevronRight size={13} /> : <ChevronLeft size={13} />}
      </button>
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

        {visited.has('docker') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'docker'}>
            <DockerPanel tabId={tab.id} serverId={tab.serverId} />
          </div>
        )}

        {visited.has('website') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'website'}>
            <WebsitePanel serverId={tab.serverId} />
          </div>
        )}

        {visited.has('database') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'database'}>
            <DatabaseManagerPanel serverId={tab.serverId} />
          </div>
        )}

        {visited.has('services') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'services'}>
            <ServicesPanel serverId={tab.serverId} />
          </div>
        )}

        {visited.has('firewall') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'firewall'}>
            <FirewallPanel serverId={tab.serverId} />
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
