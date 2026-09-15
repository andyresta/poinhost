import {
  Code2,
  Lock,
  FolderOpen,
  ScrollText,
  Route,
  Database,
  Clock,
  Globe,
  FolderLock,
  type LucideIcon,
} from 'lucide-react';

// Label fitur sengaja TIDAK diterjemahkan: semuanya nama teknologi/protokol
// (PHP, SSL, Files, Logs, Proxy, Database, Cron, DNS, SFTP) yang dipakai
// apa adanya di kedua bahasa.
export type FeatureKey = 'php' | 'ssl' | 'files' | 'logs' | 'proxy' | 'database' | 'cron' | 'dns' | 'sftp';

// FEATURES: daftar ikon menu dashboard domain (model Plesk). Dipakai DUA
// tempat dengan sumber yang sama supaya tidak pernah beda isi/urutan:
// (1) baris domain di WebsitePanel — gear membuka grid ini INLINE di bawah
// barisnya, dan (2) DomainDetailPanel sebagai menu utama domain tsb.
//
// dns ditandai hideOnSubdomain karena DNS hanya relevan di domain induk.
export const FEATURES: { key: FeatureKey; label: string; Icon: LucideIcon; hideOnSubdomain?: boolean }[] = [
  { key: 'php', label: 'PHP', Icon: Code2 },
  { key: 'ssl', label: 'SSL', Icon: Lock },
  { key: 'files', label: 'Files', Icon: FolderOpen },
  { key: 'logs', label: 'Logs', Icon: ScrollText },
  { key: 'proxy', label: 'Proxy', Icon: Route },
  { key: 'database', label: 'Database', Icon: Database },
  { key: 'cron', label: 'Cron', Icon: Clock },
  { key: 'dns', label: 'DNS', Icon: Globe, hideOnSubdomain: true },
  { key: 'sftp', label: 'SFTP', Icon: FolderLock },
];

export function visibleFeatures(isSubdomain?: boolean) {
  return FEATURES.filter((f) => !(f.hideOnSubdomain && isSubdomain));
}

export function featureLabel(key: FeatureKey) {
  return FEATURES.find((f) => f.key === key)?.label ?? key;
}

// DomainFeatureGrid grid ikon fitur satu domain. Memilih ikon TIDAK
// mengurus navigasi sendiri — pemanggil yang menentukan tujuannya (buka
// halaman fitur di panel detail).
export function DomainFeatureGrid({
  isSubdomain,
  onSelect,
}: {
  isSubdomain?: boolean;
  onSelect: (key: FeatureKey) => void;
}) {
  return (
    <div className="domain-feature-grid">
      {visibleFeatures(isSubdomain).map(({ key, label, Icon }) => (
        <button key={key} className="domain-feature-tile" onClick={() => onSelect(key)}>
          <span className="domain-feature-tile__icon">
            <Icon size={22} />
          </span>
          <span className="domain-feature-tile__label">{label}</span>
        </button>
      ))}
    </div>
  );
}
