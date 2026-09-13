import { useState } from 'react';
import { X } from 'lucide-react';

const ROWS: { label: string; bit: number }[] = [
  { label: 'Baca', bit: 4 },
  { label: 'Tulis', bit: 2 },
  { label: 'Eksekusi', bit: 1 },
];
const COLS = ['Owner', 'Group', 'Other'];

// Mengurai string mode Unix (mis. "-rw-r--r--" dari FileEntry.mode, atau
// "drwxr-xr-x" untuk direktori) jadi 3 angka oktal [owner, group, other].
function parseMode(mode: string): [number, number, number] {
  const perm = mode.length >= 10 ? mode.slice(-9) : mode.padStart(9, '-');
  const groups = [perm.slice(0, 3), perm.slice(3, 6), perm.slice(6, 9)];
  return groups.map((g) => {
    let n = 0;
    if (g[0] === 'r') n += 4;
    if (g[1] === 'w') n += 2;
    if (g[2] === 'x' || g[2] === 's' || g[2] === 't') n += 1;
    return n;
  }) as [number, number, number];
}

// Grid checkbox read/write/execute x owner/group/other — lebih enak dipakai
// daripada kotak teks oktal polos ala homepoin, tapi tetap menampilkan
// angka oktal hasilnya untuk yang sudah hafal (mis. "755", "644").
export function ChmodModal({
  path,
  currentMode,
  onConfirm,
  onClose,
}: {
  path: string;
  currentMode: string;
  onConfirm: (octal: string) => void | Promise<void>;
  onClose: () => void;
}) {
  const [bits, setBits] = useState<[number, number, number]>(() => parseMode(currentMode));
  const [busy, setBusy] = useState(false);

  function toggle(col: number, bit: number) {
    setBits((b) => {
      const next = [...b] as [number, number, number];
      next[col] ^= bit;
      return next;
    });
  }

  const octal = bits.map(String).join('');

  async function handleConfirm() {
    setBusy(true);
    try {
      await onConfirm(octal);
      onClose();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--small">
        <div className="modal-card__header">
          <h2>Ubah Permission</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body">
          <p className="chmod-path">{path}</p>
          <table className="chmod-table">
            <thead>
              <tr>
                <th />
                {COLS.map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {ROWS.map((row) => (
                <tr key={row.label}>
                  <td>{row.label}</td>
                  {[0, 1, 2].map((col) => (
                    <td key={col}>
                      <input
                        type="checkbox"
                        checked={(bits[col] & row.bit) !== 0}
                        onChange={() => toggle(col, row.bit)}
                      />
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          <div className="chmod-octal">
            Oktal: <code>{octal}</code>
          </div>
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Batal
          </button>
          <button className="btn btn--primary" disabled={busy} onClick={() => void handleConfirm()}>
            Terapkan
          </button>
        </div>
      </div>
    </div>
  );
}
