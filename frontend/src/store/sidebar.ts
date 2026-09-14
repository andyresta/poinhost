// Preferensi collapse sidebar server — sama seperti store/theme.ts, murni
// state lokal browser (localStorage), tidak lewat backend sama sekali.
import { create } from 'zustand';

const STORAGE_KEY = 'poinhost:sidebar-collapsed';

function readStoredCollapsed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

interface SidebarState {
  collapsed: boolean;
  toggle: () => void;
}

export const useSidebarStore = create<SidebarState>((set, get) => ({
  collapsed: readStoredCollapsed(),
  toggle: () => {
    const next = !get().collapsed;
    try {
      localStorage.setItem(STORAGE_KEY, next ? '1' : '0');
    } catch {
      // abaikan — preferensi cuma tidak akan tersimpan untuk sesi berikutnya
    }
    set({ collapsed: next });
  },
}));
