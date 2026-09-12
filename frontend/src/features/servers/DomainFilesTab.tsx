import { useEffect, useState } from 'react';
import { GetWebsiteDomainRoot } from '../../../wailsjs/go/main/App';
import { FilesPanel } from './FilesPanel';

// Tab Files milik satu domain di menu Website — file manager PENUH (reuse
// FilesPanel yang sama dipakai modul Files server-wide), cuma dikunci ke
// document root domain ini (tidak bisa naik ke atasnya, lihat prop
// `rootPath` di FilesPanel.tsx) supaya tidak sengaja mengubah file domain
// lain atau file sistem dari tab yang seharusnya cuma untuk satu situs.
export function DomainFilesTab({ serverId, domain }: { serverId: string; domain: string }) {
  const [root, setRoot] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    GetWebsiteDomainRoot(serverId, domain)
      .then((r) => !cancelled && setRoot(r))
      .catch((e) => !cancelled && setError(String(e)));
    return () => {
      cancelled = true;
    };
  }, [serverId, domain]);

  if (error) return <p className="overview__error">{error}</p>;
  if (!root) return <p className="workspace__placeholder">Memuat…</p>;

  return <FilesPanel serverId={serverId} rootPath={root} />;
}
