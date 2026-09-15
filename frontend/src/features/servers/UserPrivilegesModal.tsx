import { useState } from 'react';
import { X } from 'lucide-react';
import { website } from '../../../wailsjs/go/models';

// Dialog "Ubah privilege" satu user database.
//
// Menggantikan tombol lama "Terapkan ulang grants" yang membingungkan:
// privilege-nya dulu diambil diam-diam dari centang di form "+ User Baru"
// (form lain, bisa dalam keadaan tertutup), dan backend-nya cuma GRANT
// tanpa REVOKE sehingga hak tidak pernah bisa DITURUNKAN. Di sini centangnya
// berangkat dari kondisi NYATA user (DBUserInfo.privileges hasil introspeksi
// server) dan penerapannya menyetel ulang — lihat applyGrants.
//
// Cakupan database sengaja hanya DITAMPILKAN, tidak bisa diubah di sini:
// menambah/mencabut database adalah aksi terpisah yang eksplisit ("Assign
// user" dan tombol × di tab Data), supaya dialog ini tidak bisa lagi diam-
// diam memperluas akses seperti versi sebelumnya.
export function UserPrivilegesModal({
  user,
  engine,
  options,
  busy,
  onClose,
  onApply,
}: {
  user: website.DBUserInfo;
  engine: 'mysql' | 'postgresql';
  options: website.DBPrivilegeOption[];
  busy: boolean;
  onClose: () => void;
  onApply: (privileges: string[]) => void;
}) {
  const [selected, setSelected] = useState<string[]>(user.privileges ?? []);

  function toggle(key: string) {
    setSelected((p) => (p.includes(key) ? p.filter((x) => x !== key) : [...p, key]));
  }

  const scope = user.allDatabases ? 'semua database' : (user.databases ?? []).join(', ');
  const noScope = !user.allDatabases && (user.databases ?? []).length === 0;

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--small">
        <div className="modal-card__header">
          <h2>Ubah privilege — {user.username}</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body">
          <div>
            <div className="form-section">
              <h3>Berlaku pada</h3>
              {noScope ? (
                <p className="overview__error" style={{ margin: 0 }}>
                  User ini belum punya akses ke database mana pun. Beri akses dulu lewat <strong>Assign user</strong> di
                  tab Data.
                </p>
              ) : (
                <p style={{ margin: 0, fontSize: 13 }}>{scope}</p>
              )}
              <p className="chmod-path" style={{ margin: '4px 0 0' }}>
                Cakupan database tidak diubah di sini — pakai <strong>Assign user</strong> / tombol × di tab Data.
              </p>
            </div>

            <div className="form-section" style={{ marginTop: 14 }}>
              <h3>Privilege</h3>
              <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
                {options.map((p) => (
                  <label key={p.key} className="form-check" style={{ marginTop: 0 }}>
                    <input type="checkbox" checked={selected.includes(p.key)} onChange={() => toggle(p.key)} />
                    <span>{p.label}</span>
                  </label>
                ))}
              </div>
              {engine === 'postgresql' && (
                <p className="chmod-path" style={{ margin: '8px 0 0' }}>
                  PostgreSQL tidak punya grant untuk ALTER/DROP — keduanya melekat pada kepemilikan objek, jadi centang itu
                  diabaikan. Untuk memberikannya, jadikan user ini pemilik database.
                </p>
              )}
              {engine === 'mysql' && user.hosts && user.hosts.length > 1 && (
                <p className="chmod-path" style={{ margin: '8px 0 0' }}>
                  Diterapkan ke semua host user ini: {user.hosts.join(', ')}.
                </p>
              )}
            </div>
          </div>
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Batal
          </button>
          <button className="btn btn--primary" disabled={busy || noScope} onClick={() => onApply(selected)}>
            {busy && <span className="spinner" />} Terapkan
          </button>
        </div>
      </div>
    </div>
  );
}
