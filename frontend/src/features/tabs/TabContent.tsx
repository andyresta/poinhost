import { useTabsStore } from '../../store/tabs';
import { ServerWorkspace } from '../servers/ServerWorkspace';
import { MigrationWorkspace } from '../migration/MigrationWorkspace';

// Semua tab yang terbuka tetap MOUNTED di DOM sekaligus — yang berubah
// cuma `hidden` (lewat CSS, bukan unmount/remount React). Ini yang membuat
// pindah tab instan: state scroll, path file manager, buffer terminal, dsb
// di tab non-aktif tidak hilang dan tidak perlu di-fetch ulang saat tab itu
// difokuskan lagi — beda dengan homepoin yang full page reload setiap
// pindah menu (lihat ARCHITECTURE.md, bagian "Kenapa switching modul di
// homepoin terasa lambat").
export function TabContent() {
  const { tabs, activeTabId } = useTabsStore();

  if (tabs.length === 0) {
    return (
      <div className="tab-content__empty">
        Belum ada tab terbuka. Pilih server di panel kiri untuk mulai.
      </div>
    );
  }

  return (
    <div className="tab-content">
      {tabs.map((tab) => (
        <div key={tab.id} hidden={tab.id !== activeTabId} className="tab-content__panel">
          {tab.kind === 'migration' ? (
            <MigrationWorkspace tab={tab} />
          ) : (
            <ServerWorkspace tab={tab} />
          )}
        </div>
      ))}
    </div>
  );
}
