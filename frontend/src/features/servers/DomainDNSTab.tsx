import { useEffect, useState } from 'react';
import { Clipboard, Download } from 'lucide-react';
import { GetWebsiteDNSPreview, ExportWebsiteDNSZone } from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';

// Tab DNS — HANYA generator file zona BIND untuk di-import manual ke
// provider DNS (mis. Cloudflare); poinhost (sama seperti homepoin) TIDAK
// menjalankan DNS server apa pun di VPS. Murni preview + salin/unduh, tidak
// ada state yang perlu disimpan sama sekali.
export function DomainDNSTab({ serverId, domain }: { serverId: string; domain: string }) {
  const [preview, setPreview] = useState<website.DNSPreviewResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    GetWebsiteDNSPreview(serverId, domain)
      .then(setPreview)
      .catch((e) => setError(String(e)));
  }, [serverId, domain]);

  async function handleCopy() {
    if (!preview) return;
    await navigator.clipboard.writeText(preview.zoneText);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  async function handleDownload() {
    try {
      await ExportWebsiteDNSZone(serverId, domain);
    } catch (e) {
      setError(String(e));
    }
  }

  if (error) return <p className="overview__error">{error}</p>;
  if (!preview) return <p className="workspace__placeholder">Memuat…</p>;

  return (
    <div>
      <p className="chmod-path">
        IP server: <code>{preview.serverHost || '(kosong)'}</code>
        {!preview.hostValid && ' — bukan alamat IP publik yang valid, record di bawah kemungkinan tidak berguna.'}
      </p>

      <table className="files-panel__table">
        <thead>
          <tr>
            <th>Tipe</th>
            <th>Nama</th>
            <th>Isi</th>
            <th>TTL</th>
          </tr>
        </thead>
        <tbody>
          {preview.records.map((r, i) => (
            <tr key={i}>
              <td>{r.type}</td>
              <td>{r.name}</td>
              <td>{r.content}</td>
              <td>{r.ttl}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <pre className="docker-engine__log" style={{ marginTop: 10 }}>
        {preview.zoneText}
      </pre>

      <div className="files-panel__toolbar" style={{ marginTop: 8 }}>
        <button className="btn btn--sm" onClick={() => void handleCopy()}>
          {copied ? 'Tersalin!' : (<><Clipboard size={13} /> Salin</>)}
        </button>
        <button className="btn btn--sm btn--primary" disabled={!preview.hostValid} onClick={() => void handleDownload()}>
          <Download size={13} /> Unduh (.zone)
        </button>
      </div>
    </div>
  );
}
