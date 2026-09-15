import { useEffect } from 'react';
import './App.css';
import { useTabsStore } from './store/tabs';
import { ServersPage } from './features/servers/ServersPage';
import { TabBar } from './features/tabs/TabBar';
import { TabContent } from './features/tabs/TabContent';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { servers } from '../wailsjs/go/models';
import { useSidebarStore } from './store/sidebar';
import { ConfirmProvider } from './components/ConfirmDialog';
import { I18nProvider, useT } from './i18n';
import { ChevronLeft, ChevronRight } from 'lucide-react';

// Handle ciutkan/perluas diletakkan PERSIS di garis pemisah sidebar (bukan
// tombol di dalam header panel) — pola umum IDE: gagang kecil yang menempel
// di divider, jadi tidak memakan ruang header dan jelas mengacu ke panel
// mana. Dirender sebagai anak #App (bukan di dalam <aside>) supaya tidak
// terpotong overflow-y:auto milik sidebar.
function SidebarEdgeToggle() {
  const t = useT();
  const collapsed = useSidebarStore((s) => s.collapsed);
  const toggle = useSidebarStore((s) => s.toggle);
  return (
    <button
      className={`edge-toggle app__edge-toggle${collapsed ? ' app__edge-toggle--collapsed' : ''}`}
      title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
      aria-label={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
      onClick={toggle}
    >
      {collapsed ? <ChevronRight size={13} /> : <ChevronLeft size={13} />}
    </button>
  );
}

function App() {
  const collapsed = useSidebarStore((s) => s.collapsed);
  const loadTabs = useTabsStore((s) => s.loadTabs);
  const loadServers = useTabsStore((s) => s.loadServers);
  const loadStatuses = useTabsStore((s) => s.loadStatuses);
  const applyStatus = useTabsStore((s) => s.applyStatus);

  // Restore layout tab dari sesi sebelumnya (ui_tabs di SQLite) begitu
  // aplikasi dibuka — mirip "restore tabs" browser.
  useEffect(() => {
    void loadServers();
    void loadTabs();
    void loadStatuses();
  }, [loadServers, loadTabs, loadStatuses]);

  // Status server (online/offline, CPU/RAM/disk) di-PUSH oleh backend lewat
  // event Wails, bukan di-poll dari sini — lihat Collector di
  // internal/modules/servers/collector.go. Satu listener untuk seluruh app,
  // dipasang sekali di root, bukan per-komponen yang menampilkan status.
  useEffect(() => {
    return EventsOn('server:status', (raw: unknown) => {
      applyStatus(servers.ServerStatus.createFrom(raw));
    });
  }, [applyStatus]);

  return (
    <I18nProvider>
      <ConfirmProvider>
      <div id="App">
        <aside className={`app__sidebar${collapsed ? ' app__sidebar--collapsed' : ''}`}>
          <ServersPage />
        </aside>
        <SidebarEdgeToggle />
        <main className="app__main">
          <TabBar />
          <TabContent />
        </main>
      </div>
      </ConfirmProvider>
    </I18nProvider>
  );
}

export default App;
