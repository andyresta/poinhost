// Preferensi ciutkan nav modul di dalam tab (Overview/Website/Database/…)
// — sama polanya dengan store/sidebar.ts untuk sidebar server: murni state
// lokal browser, disimpan di localStorage, tidak lewat backend.
import { create } from 'zustand';

const STORAGE_KEY = 'poinhost:workspace-nav-collapsed';

function readStoredCollapsed(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

interface WorkspaceNavState {
  collapsed: boolean;
  toggle: () => void;
}

export const useWorkspaceNavStore = create<WorkspaceNavState>((set, get) => ({
  collapsed: readStoredCollapsed(),
  toggle: () => {
    const next = !get().collapsed;
    try {
      localStorage.setItem(STORAGE_KEY, next ? '1' : '0');
    } catch {
      // abaikan — preferensi cuma tidak tersimpan untuk sesi berikutnya
    }
    set({ collapsed: next });
  },
}));
