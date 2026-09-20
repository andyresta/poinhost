import { useEffect, useState } from 'react';
import { FolderSync, Database, Boxes, ChevronLeft, ChevronRight } from 'lucide-react';
import { useTabsStore } from '../../store/tabs';
import { useWorkspaceNavStore } from '../../store/workspaceNav';
import { useT } from '../../i18n';
import { FileTransferPanel } from './FileTransferPanel';
import { DockerMigrationPanel } from './DockerMigrationPanel';
import { DBMigrationPanel } from './DBMigrationPanel';
import type { session } from '../../../wailsjs/go/models';

// Menu di dalam tab migrasi. Bentuknya sengaja sama persis dengan nav modul
// di tab server (ServerWorkspace) — sama-sama "pilih bagian di dalam satu
// tab", jadi tidak ada gunanya memperkenalkan pola navigasi kedua.
const MODULES = [
  { key: 'migration-files', labelKey: 'migration.nav.files', Icon: FolderSync },
  { key: 'migration-database', labelKey: 'migration.nav.database', Icon: Database },
  { key: 'migration-docker', labelKey: 'migration.nav.docker', Icon: Boxes },
] as const;

// Isi satu tab migrasi. Panel Files tetap MOUNTED begitu pernah dibuka —
// di dalamnya ada folder yang sedang dijelajahi di dua server sekaligus dan
// mungkin transfer yang sedang berjalan, semuanya hilang kalau di-unmount
// hanya karena user melirik menu sebelah.
export function MigrationWorkspace({ tab }: { tab: session.Tab }) {
  const t = useT();
  const navCollapsed = useWorkspaceNavStore((s) => s.collapsed);
  const toggleNav = useWorkspaceNavStore((s) => s.toggle);
  const setModule = useTabsStore((s) => s.setModule);
  const [visited, setVisited] = useState<Set<string>>(() => new Set([tab.activeModule]));

  useEffect(() => {
    if (!visited.has(tab.activeModule)) {
      setVisited((v) => new Set(v).add(tab.activeModule));
    }
  }, [tab.activeModule, visited]);

  const activeMeta = MODULES.find((m) => m.key === tab.activeModule);

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
        <h2>{activeMeta ? t(activeMeta.labelKey) : t('migration.title')}</h2>

        {visited.has('migration-files') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'migration-files'}>
            <FileTransferPanel />
          </div>
        )}

        {visited.has('migration-docker') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'migration-docker'}>
            <DockerMigrationPanel />
          </div>
        )}

        {visited.has('migration-database') && (
          <div className="workspace__module" hidden={tab.activeModule !== 'migration-database'}>
            <DBMigrationPanel />
          </div>
        )}
      </div>
    </div>
  );
}
