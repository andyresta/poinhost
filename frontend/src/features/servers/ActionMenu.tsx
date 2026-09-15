import { useEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { MoreVertical } from 'lucide-react';

// Menu aksi ringkas (tombol titik-tiga) untuk baris tabel — dipakai supaya
// satu baris tidak penuh tombol ikon berjajar/bertumpuk.
//
// Isinya dirender lewat PORTAL ke <body> dengan posisi fixed, bukan absolute
// di dalam baris: tabel panjang dibungkus container ber-overflow (scroll),
// dan container semacam itu MEMOTONG anak absolute-nya — begitu juga kalau
// overflow-x dibiarkan "visible", karena browser memaksa kedua sumbu jadi
// "auto" saat salah satunya bukan visible. Pola yang sama sudah dipakai
// popover kartu server di ServersPage.
export function ActionMenu({ title = 'Aksi', children }: { title?: string; children: (close: () => void) => ReactNode }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState({ top: 0, left: 0 });
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    function onPointerDown(e: MouseEvent) {
      const t = e.target as Node;
      if (triggerRef.current?.contains(t) || menuRef.current?.contains(t)) return;
      setOpen(false);
    }
    // Posisi menu dihitung sekali saat dibuka; begitu ada scroll/resize
    // posisinya jadi basi — lebih jujur menutupnya daripada menampilkan
    // menu yang "melayang" lepas dari barisnya.
    function onReflow() {
      setOpen(false);
    }
    document.addEventListener('mousedown', onPointerDown);
    window.addEventListener('scroll', onReflow, true);
    window.addEventListener('resize', onReflow);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      window.removeEventListener('scroll', onReflow, true);
      window.removeEventListener('resize', onReflow);
    };
  }, [open]);

  function toggle() {
    const el = triggerRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    setPos({ top: rect.bottom + 4, left: rect.right });
    setOpen((o) => !o);
  }

  return (
    <>
      <button ref={triggerRef} title={title} onClick={toggle}>
        <MoreVertical size={14} />
      </button>
      {open &&
        createPortal(
          <div ref={menuRef} className="action-menu" style={{ top: pos.top, left: pos.left }}>
            {children(() => setOpen(false))}
          </div>,
          document.body,
        )}
    </>
  );
}
