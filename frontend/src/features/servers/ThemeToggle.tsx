import { useThemeStore } from '../../store/theme';

// Ikon SVG polos (bukan emoji) — matahari/bulan, satu ikon yang menunjukkan
// tema TUJUAN kalau diklik (bukan tema yang sedang aktif), pola umum
// toggle tema.
function SunIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2.5M12 19.5V22M4.9 4.9l1.8 1.8M17.3 17.3l1.8 1.8M2 12h2.5M19.5 12H22M4.9 19.1l1.8-1.8M17.3 6.7l1.8-1.8" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 14.5a8.5 8.5 0 1 1-9.5-11.9 7 7 0 0 0 9.5 11.9z" />
    </svg>
  );
}

export function ThemeToggle() {
  const mode = useThemeStore((s) => s.mode);
  const toggle = useThemeStore((s) => s.toggle);
  const isDark = mode === 'dark';

  return (
    <button
      className="btn btn--sm theme-toggle"
      title={isDark ? 'Pakai tema terang' : 'Pakai tema gelap'}
      aria-label={isDark ? 'Pakai tema terang' : 'Pakai tema gelap'}
      onClick={toggle}
    >
      {isDark ? <SunIcon /> : <MoonIcon />}
    </button>
  );
}
