// Baris output yang dialirkan dari server lewat PTY.
//
// sshpool menandai baris yang berakhir dengan carriage return — bar progress
// dnf, apt, docker pull, certbot — dengan prefiks tak terlihat. Penanda itu
// dulu dipancarkan tanpa ada satu pun yang membacanya, sehingga teks
// "poinhostPROG" ikut terbaca di log instalasi yang dilihat user.
//
// Di terminal, carriage return mengembalikan kursor ke awal baris: apa pun
// yang datang sesudahnya MENIMPA baris itu, bukan menambah baris baru. Dua
// fungsi di bawah ini yang membuat log di app berperilaku sama.

/** Harus sama persis dengan sshpool.ProgressLinePrefix. */
export const PROGRESS_PREFIX = '\x00\x01poinhostPROG\x01\x00';

/**
 * Menambahkan satu baris aliran, dengan perilaku carriage return.
 *
 * Baris yang MENGIKUTI baris progress menimpanya — termasuk baris biasa,
 * karena itulah keadaan akhir baris terminal yang sama. Tanpa ini, satu
 * unduhan paket meninggalkan ratusan baris nyaris identik yang mengubur
 * pesan galat di antaranya.
 */
export function pushStreamLine(lines: string[], raw: string): string[] {
  const timpa = lines.length > 0 && lines[lines.length - 1].startsWith(PROGRESS_PREFIX);
  return timpa ? [...lines.slice(0, -1), raw] : [...lines, raw];
}

/** Menggabungkan baris untuk ditampilkan, tanpa penanda progressnya. */
export function renderStreamLines(lines: string[]): string {
  return lines
    .map((l) => (l.startsWith(PROGRESS_PREFIX) ? l.slice(PROGRESS_PREFIX.length) : l))
    .join('\n');
}
