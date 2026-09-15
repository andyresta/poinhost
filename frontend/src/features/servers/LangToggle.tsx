import { useI18n } from '../../i18n';

// Bendera digambar sebagai SVG, bukan emoji (🇮🇩/🇬🇧): Windows tidak
// mengirimkan font emoji bendera sama sekali, jadi emoji-nya cuma tampil
// sebagai dua huruf kotak ("ID"/"GB") — sementara aplikasi ini justru
// dipakai di Windows.
function FlagID() {
  return (
    <svg width="18" height="13" viewBox="0 0 18 13" aria-hidden="true">
      <rect width="18" height="6.5" fill="#e70011" />
      <rect y="6.5" width="18" height="6.5" fill="#fff" />
    </svg>
  );
}

function FlagGB() {
  return (
    <svg width="18" height="13" viewBox="0 0 60 40" aria-hidden="true">
      <rect width="60" height="40" fill="#012169" />
      <path d="M0,0 L60,40 M60,0 L0,40" stroke="#fff" strokeWidth="8" />
      <path d="M0,0 L60,40 M60,0 L0,40" stroke="#C8102E" strokeWidth="4" />
      <path d="M30,0 V40 M0,20 H60" stroke="#fff" strokeWidth="13" />
      <path d="M30,0 V40 M0,20 H60" stroke="#C8102E" strokeWidth="8" />
    </svg>
  );
}

// Toggle bahasa — benderanya menunjukkan bahasa yang SEDANG AKTIF (bendera
// Inggris = UI sedang berbahasa Inggris), BUKAN bahasa tujuan seperti
// ThemeToggle. Sengaja beda dari toggle tema: ikon matahari/bulan tidak
// punya makna "keadaan sekarang" yang kuat, sedangkan bendera langsung
// terbaca sebagai "ini bahasa yang dipakai". Tujuan kliknya tetap
// dijelaskan lewat tooltip.
export function LangToggle() {
  const { lang, setLang, t } = useI18n();
  const isEN = lang === 'en';

  return (
    <button
      className="btn btn--sm theme-toggle"
      title={isEN ? t('sidebar.lang.toID') : t('sidebar.lang.toEN')}
      aria-label={isEN ? t('sidebar.lang.toID') : t('sidebar.lang.toEN')}
      onClick={() => setLang(isEN ? 'id' : 'en')}
    >
      {isEN ? <FlagGB /> : <FlagID />}
    </button>
  );
}
