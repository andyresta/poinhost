import { useTabsStore } from '../../store/tabs';

// Tab bar ala browser: satu tab = satu server yang sedang dikelola. Klik
// tab lain hanya ganti `activeTabId` di store — TIDAK unmount/reload apapun
// (lihat TabContent.tsx), jadi pindah tab terasa instan walau tab itu
// menunjuk server yang berbeda dengan koneksi SSH yang berbeda pula.
export function TabBar() {
  const { tabs, activeTabId, servers, setActiveTab, closeTab } = useTabsStore();

  return (
    <div className="tab-bar" role="tablist">
      {tabs.map((tab) => {
        const server = servers.find((s) => s.id === tab.serverId);
        const isActive = tab.id === activeTabId;
        return (
          <div
            key={tab.id}
            role="tab"
            aria-selected={isActive}
            className={`tab-bar__item${isActive ? ' tab-bar__item--active' : ''}`}
            onClick={() => setActiveTab(tab.id)}
          >
            <span
              className="tab-bar__dot"
              style={{ backgroundColor: server?.color ?? 'var(--accent)' }}
            />
            <span className="tab-bar__title">{tab.title || server?.name || tab.serverId}</span>
            <button
              className="tab-bar__close"
              aria-label="Tutup tab"
              onClick={(e) => {
                e.stopPropagation();
                void closeTab(tab.id);
              }}
            >
              ×
            </button>
          </div>
        );
      })}
    </div>
  );
}
