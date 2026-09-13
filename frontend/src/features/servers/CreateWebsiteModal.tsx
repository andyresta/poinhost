import { useEffect, useState } from 'react';
import { CreateWebsite, GetWebsitePHPStatus } from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { X } from 'lucide-react';

const SSL_EMAIL_KEY = 'poinhost.website.sslEmail';

// Alur "Buat Website" gabungan: domain + versi PHP (opsional) + SSL
// (opsional) dalam SATU submit — perbaikan UX dari homepoin yang memaksa
// tiga kunjungan halaman terpisah (Domains -> PHP -> SSL) untuk hasil yang
// sama. Kegagalan PHP/SSL (keduanya opsional) dilaporkan sebagai warning,
// BUKAN membatalkan domain yang sudah berhasil dibuat (lihat
// website.Service.CreateWebsite).
export function CreateWebsiteModal({
  serverId,
  onClose,
  onDone,
}: {
  serverId: string;
  onClose: () => void;
  onDone: (result: website.CreateWebsiteResult) => void;
}) {
  const [domain, setDomain] = useState('');
  const [phpVersions, setPhpVersions] = useState<string[]>([]);
  const [phpVersion, setPhpVersion] = useState('');
  const [enableSsl, setEnableSsl] = useState(false);
  const [sslEmail, setSslEmail] = useState(() => localStorage.getItem(SSL_EMAIL_KEY) ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    GetWebsitePHPStatus(serverId, '')
      .then((st) => setPhpVersions(st.versions.map((v) => v.version)))
      .catch(() => setPhpVersions([]));
  }, [serverId]);

  async function handleSubmit() {
    if (!domain.trim()) return;
    if (enableSsl && !sslEmail.trim()) {
      setError('Email wajib diisi untuk menerbitkan sertifikat SSL');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      if (enableSsl) localStorage.setItem(SSL_EMAIL_KEY, sslEmail.trim());
      const result = await CreateWebsite(
        new website.CreateWebsiteRequest({
          serverId,
          domain: domain.trim(),
          phpVersion: phpVersion || undefined,
          enableSsl,
          sslEmail: enableSsl ? sslEmail.trim() : undefined,
        }),
      );
      onDone(result);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card">
        <div className="modal-card__header">
          <h2>Buat Website</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body">
          <label className="form-field">
            <span>Nama domain</span>
            <input autoFocus placeholder="contoh.com" value={domain} onChange={(e) => setDomain(e.target.value)} />
          </label>

          <label className="form-field">
            <span>PHP (opsional)</span>
            <select value={phpVersion} onChange={(e) => setPhpVersion(e.target.value)}>
              <option value="">Static saja (tanpa PHP)</option>
              {phpVersions.map((v) => (
                <option key={v} value={v}>
                  PHP {v}
                </option>
              ))}
            </select>
            {phpVersions.length === 0 && (
              <span className="chmod-path" style={{ margin: '4px 0 0' }}>
                Belum ada PHP-FPM terpasang — bisa dipasang nanti dari tab PHP setelah website dibuat.
              </span>
            )}
          </label>

          <label className="form-check">
            <input type="checkbox" checked={enableSsl} onChange={(e) => setEnableSsl(e.target.checked)} />
            <span>Aktifkan SSL (Let's Encrypt) sekarang</span>
          </label>

          {enableSsl && (
            <label className="form-field">
              <span>Email (untuk Let's Encrypt)</span>
              <input
                type="email"
                placeholder="admin@contoh.com"
                value={sslEmail}
                onChange={(e) => setSslEmail(e.target.value)}
              />
            </label>
          )}

          {enableSsl && (
            <p className="chmod-path">
              Pastikan DNS domain ini sudah diarahkan ke IP server SEBELUM submit — penerbitan sertifikat
              butuh domain sudah bisa diakses dari internet (validasi HTTP-01).
            </p>
          )}

          {error && <p className="overview__error">{error}</p>}
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Batal
          </button>
          <button className="btn btn--primary" disabled={busy || !domain.trim()} onClick={() => void handleSubmit()}>
            {busy ? 'Membuat…' : 'Buat Website'}
          </button>
        </div>
      </div>
    </div>
  );
}
