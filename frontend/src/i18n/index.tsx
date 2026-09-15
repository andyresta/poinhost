import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { messages, type MessageKey } from './messages';

export type Lang = 'en' | 'id';

const STORAGE_KEY = 'poinhost:lang';

// Default SENGAJA 'en' (permintaan eksplisit): aplikasi ini awalnya ditulis
// penuh bahasa Indonesia, tapi tampilan pertama untuk pengguna baru harus
// bahasa Inggris. Preferensi tersimpan lokal saja (localStorage), sama
// seperti tema — tidak ada alasan menyimpan pilihan bahasa di server.
function readStoredLang(): Lang {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'en' || v === 'id') return v;
  } catch {
    // localStorage tidak tersedia — pakai default.
  }
  return 'en';
}

type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;

interface I18nValue {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: Translate;
}

const I18nContext = createContext<I18nValue | null>(null);

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n dipakai di luar <I18nProvider>');
  return ctx;
}

/** useT pintasan kalau komponen cuma butuh fungsi terjemahannya. */
export function useT(): Translate {
  return useI18n().t;
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(readStoredLang);

  const setLang = useCallback((l: Lang) => {
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch {
      // abaikan — pilihan cuma tidak tersimpan untuk sesi berikutnya
    }
    setLangState(l);
  }, []);

  const t = useCallback<Translate>(
    (key, vars) => {
      // Fallback berlapis: bahasa aktif -> Inggris -> key mentah. Key mentah
      // sengaja ditampilkan apa adanya (bukan string kosong) supaya teks
      // yang belum diterjemahkan kelihatan jelas saat dipakai, bukan hilang
      // diam-diam.
      const raw = messages[lang]?.[key] ?? messages.en[key] ?? key;
      if (!vars) return raw;
      return raw.replace(/\{(\w+)\}/g, (m, name: string) => (name in vars ? String(vars[name]) : m));
    },
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}
