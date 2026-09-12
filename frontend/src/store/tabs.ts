// Zustand store untuk state tab & server. Ini satu-satunya sumber
// kebenaran untuk "tab mana yang terbuka & modul apa yang aktif di
// dalamnya" di sisi frontend — dipakai oleh TabBar (render daftar tab)
// dan TabContent (render panel konten per tab, lihat catatan keep-alive
// di TabContent.tsx untuk kenapa switching tab harus instan).
import { create } from 'zustand';
import {
  ListServers,
  SaveServer,
  DeleteServer,
  OpenServerTab,
  CloseServerTab,
  ListServerTabs,
  SetTabActiveModule,
  ReorderServerTabs,
} from '../../wailsjs/go/main/App';
import { session, type servers } from '../../wailsjs/go/models';

interface TabsState {
  servers: servers.Server[];
  tabs: session.Tab[];
  activeTabId: string | null;
  loading: boolean;

  loadServers: () => Promise<void>;
  loadTabs: () => Promise<void>;
  saveServer: (req: servers.SaveServerRequest) => Promise<void>;
  deleteServer: (id: string) => Promise<void>;

  openTab: (serverId: string, title: string) => Promise<void>;
  closeTab: (tabId: string) => Promise<void>;
  setActiveTab: (tabId: string) => void;
  setModule: (tabId: string, module: string) => Promise<void>;
  reorderTabs: (tabIds: string[]) => Promise<void>;
}

export const useTabsStore = create<TabsState>((set, get) => ({
  servers: [],
  tabs: [],
  activeTabId: null,
  loading: false,

  loadServers: async () => {
    const list = await ListServers();
    set({ servers: list ?? [] });
  },

  loadTabs: async () => {
    const list = await ListServerTabs();
    set({ tabs: list ?? [] });
    const current = get().activeTabId;
    if (!current && list && list.length > 0) {
      set({ activeTabId: list[0].id });
    }
  },

  saveServer: async (req) => {
    await SaveServer(req);
    await get().loadServers();
  },

  deleteServer: async (id) => {
    await DeleteServer(id);
    await get().loadServers();
  },

  // Membuka tab baru untuk server tertentu. Tab BARU selalu dibuat — kalau
  // user ingin "fokuskan tab yang sudah ada" itu keputusan UI di pemanggil
  // (mis. ServersPage bisa cek get().tabs dulu sebelum openTab), supaya
  // eksplisit boleh punya banyak tab ke server yang sama sekaligus
  // (misalnya satu tab Files, satu tab Terminal, ke server yang sama).
  openTab: async (serverId, title) => {
    const tab = await OpenServerTab(serverId, title);
    set((s) => ({ tabs: [...s.tabs, tab], activeTabId: tab.id }));
  },

  closeTab: async (tabId) => {
    await CloseServerTab(tabId);
    set((s) => {
      const tabs = s.tabs.filter((t) => t.id !== tabId);
      let activeTabId = s.activeTabId;
      if (activeTabId === tabId) {
        activeTabId = tabs.length > 0 ? tabs[tabs.length - 1].id : null;
      }
      return { tabs, activeTabId };
    });
  },

  setActiveTab: (tabId) => set({ activeTabId: tabId }),

  setModule: async (tabId, module) => {
    // Optimistic update dulu supaya UI terasa instan (bukan menunggu
    // roundtrip binding Wails selesai baru pindah panel).
    set((s) => ({
      tabs: s.tabs.map((t) =>
        t.id === tabId ? session.Tab.createFrom({ ...t, activeModule: module }) : t,
      ),
    }));
    await SetTabActiveModule(tabId, module);
  },

  reorderTabs: async (tabIds) => {
    set((s) => {
      const byId = new Map(s.tabs.map((t) => [t.id, t]));
      const tabs = tabIds.map((id) => byId.get(id)!).filter(Boolean);
      return { tabs };
    });
    await ReorderServerTabs(tabIds);
  },
}));
