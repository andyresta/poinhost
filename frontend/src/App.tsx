import { useEffect } from 'react';
import './App.css';
import { useTabsStore } from './store/tabs';
import { ServersPage } from './features/servers/ServersPage';
import { TabBar } from './features/tabs/TabBar';
import { TabContent } from './features/tabs/TabContent';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { servers } from '../wailsjs/go/models';

function App() {
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
    <div id="App">
      <aside className="app__sidebar">
        <ServersPage />
      </aside>
      <main className="app__main">
        <TabBar />
        <TabContent />
      </main>
    </div>
  );
}

export default App;
