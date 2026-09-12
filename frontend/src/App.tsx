import { useEffect } from 'react';
import './App.css';
import { useTabsStore } from './store/tabs';
import { ServersPage } from './features/servers/ServersPage';
import { TabBar } from './features/tabs/TabBar';
import { TabContent } from './features/tabs/TabContent';

function App() {
  const loadTabs = useTabsStore((s) => s.loadTabs);
  const loadServers = useTabsStore((s) => s.loadServers);

  // Restore layout tab dari sesi sebelumnya (ui_tabs di SQLite) begitu
  // aplikasi dibuka — mirip "restore tabs" browser.
  useEffect(() => {
    void loadServers();
    void loadTabs();
  }, [loadServers, loadTabs]);

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
