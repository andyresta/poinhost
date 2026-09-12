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
  ListServerStatuses,
  RefreshServerStatus,
} from '../../wailsjs/go/main/App';
import { session, servers } from '../../wailsjs/go/models';

interface TabsState {
  servers: servers.Server[];
  tabs: session.Tab[];
  activeTabId: string | null;
  loading: boolean;
  // Status per serverId — diisi sekali via loadStatuses() lalu diperbarui
  // terus-menerus lewat event "server:status" dari backend (lihat App.tsx),
  // BUKAN dengan frontend polling binding berulang-ulang.
  statuses: Record<string, servers.ServerStatus>;

  loadServers: () => Promise<void>;
  loadTabs: () => Promise<void>;
  loadStatuses: () => Promise<void>;
  applyStatus: (status: servers.ServerStatus) => void;
  refreshStatus: (serverId: string) => Promise<void>;
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
  statuses: {},

  loadServers: async () => {
    const list = await ListServers();
    set({ servers: list ?? [] });
  },

  // Ambil snapshot status TERAKHIR yang sudah diketahui backend (instan,
  // tidak menunggu SSH apa pun — Collector.Snapshot cuma baca cache).
  loadStatuses: async () => {
    const list = await ListServerStatuses();
    const statuses: Record<string, servers.ServerStatus> = {};
    for (const st of list ?? []) statuses[st.id] = st;
    set({ statuses });
  },

  applyStatus: (status) => {
    set((s) => ({ statuses: { ...s.statuses, [status.id]: status } }));
  },

  refreshStatus: async (serverId) => {
    const status = await RefreshServerStatus(serverId);
    get().applyStatus(status);
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

  // Tutup dulu semua tab yang menunjuk ke server ini (lewat closeTab, BUKAN
  // langsung hapus dari state) supaya sesi terminal dedicated-nya dibersihkan
  // rapi lewat TerminalRegistry.CloseAllForTab di backend sebelum koneksi
  // server itu sendiri ditutup total oleh Pool.UnregisterServer.
  deleteServer: async (id) => {
    const staleTabIds = get().tabs.filter((t) => t.serverId === id).map((t) => t.id);
    for (const tabId of staleTabIds) {
      await get().closeTab(tabId);
    }
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
