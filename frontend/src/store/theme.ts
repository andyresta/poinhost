// Preferensi tema (terang/gelap) — murni state lokal browser, tidak lewat
// backend Go sama sekali (beda dari store/tabs.ts yang semuanya via
// binding Wails): tidak ada alasan menyimpan preferensi tampilan di
// server/SQLite, localStorage sudah cukup dan lebih instan.
//
// applyTheme dipanggil SEKALI saat modul ini pertama di-import (bukan di
// dalam useEffect komponen) supaya atribut data-theme di <html> sudah
// benar SEBELUM render pertama — kalau menunggu useEffect, akan ada
// kedipan sesaat memakai tema default sebelum preferensi tersimpan
// diterapkan.
import { create } from 'zustand';

export type ThemeMode = 'light' | 'dark';

const STORAGE_KEY = 'poinhost:theme';

function readStoredTheme(): ThemeMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'light' || v === 'dark') return v;
  } catch {
    // localStorage tidak tersedia — jarang terjadi di app desktop, default gelap saja.
  }
  return 'dark';
}

function applyTheme(mode: ThemeMode) {
  document.documentElement.dataset.theme = mode;
}

interface ThemeState {
  mode: ThemeMode;
  toggle: () => void;
}

const initialMode = readStoredTheme();
applyTheme(initialMode);

export const useThemeStore = create<ThemeState>((set, get) => ({
  mode: initialMode,
  toggle: () => {
    const next: ThemeMode = get().mode === 'dark' ? 'light' : 'dark';
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // abaikan — preferensi cuma tidak akan tersimpan untuk sesi berikutnya
    }
    applyTheme(next);
    set({ mode: next });
  },
}));
