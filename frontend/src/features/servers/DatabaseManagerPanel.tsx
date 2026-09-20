import { useEffect, useRef, useState } from 'react';
import { CircleCheck, TriangleAlert, Boxes, Link2, KeyRound, Search, X, UserPlus, Trash2, ArrowLeft } from 'lucide-react';
import { ActionMenu } from './ActionMenu';
import { useConfirm } from '../../components/ConfirmDialog';
import { UserPrivilegesModal } from './UserPrivilegesModal';
import { useT } from '../../i18n';
import {
  GetWebsiteDBStatus,
  StartWebsiteDB,
  StreamWebsiteInstall,
  StopDockerStream,
  ListWebsiteDatabases,
  CreateWebsiteDatabase,
  ListWebsiteDatabaseUsers,
  CreateWebsiteDatabaseUser,
  SetWebsiteDatabaseGrants,
  GrantWebsiteDatabaseUser,
  RevokeWebsiteDatabaseUser,
  DropWebsiteDatabase,
  DropWebsiteDatabaseUser,
  GetWebsiteDBPrivileges,
  ListWebsiteDBCredentials,
  SaveWebsiteDBCredential,
  ForgetWebsiteDBCredential,
  ListWebsiteServerDatabaseDomains,
  SetWebsiteDatabaseDomains,
  ListWebsites,
  GetWebsiteDBDockerAccessStatus,
  EnsureWebsiteDBDockerAccess,
  DisableWebsiteDBDockerAccess,
  GetWebsiteDBVersions,
} from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { MySQLExplorer } from './MySQLExplorer';
import { PGExplorer } from './PGExplorer';

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Utilitas provisioning MySQL/PostgreSQL ringan (database/user/grants) +
// Explore (koneksi driver asli, browse tabel/baris/query — lihat
// MySQLExplorerModal/PGExplorerModal), password disimpan LOKAL di vault
// mesin ini, TIDAK pernah ditulis ke server target.
//
// Dipakai di DUA tempat: (1) tab "Database" per-domain di Website (dengan
// `domain` diisi — tambahan: bisa menautkan database ke domain itu, murni
// kurasi lokal poinhost, tidak mengubah akses); (2) modul "Database"
// top-level per server (`domain` dikosongkan) — supaya bisa dipakai
// langsung tanpa perlu server itu punya domain/website apa pun dulu, sesuai
// temuan bahwa modul ini TERNYATA bukan benar-benar domain-scoped (lihat
// komentar di internal/modules/website/database.go): MySQL/PostgreSQL
// server-wide, Domain di request backend cuma breadcrumb UI.
export function DatabaseManagerPanel({ serverId, domain }: { serverId: string; domain?: string }) {
  const confirm = useConfirm();
  const t = useT();
  const [engine, setEngine] = useState<'mysql' | 'postgresql'>('mysql');
  const [status, setStatus] = useState<website.DBEngineStatus | null>(null);
  const [databases, setDatabases] = useState<website.DBDatabaseInfo[]>([]);
  const [users, setUsers] = useState<website.DBUserInfo[]>([]);
  const [privileges, setPrivileges] = useState<website.DBPrivilegeOption[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [lines, setLines] = useState<string[]>([]);
  const streamIdRef = useRef<string | null>(null);

  const [newDbName, setNewDbName] = useState('');
  const [newDbOwner, setNewDbOwner] = useState('');
  // assignTarget: database yang sedang dibuka panel "assign user"-nya.
  const [assignTarget, setAssignTarget] = useState<string | null>(null);
  const [assignUsername, setAssignUsername] = useState('');
  const [showUserForm, setShowUserForm] = useState(false);
  const [newUser, setNewUser] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newHost, setNewHost] = useState('%');
  // Default SENGAJA tidak dicentang: "akses semua database" itu grant global
  // (MySQL ON *.*, PostgreSQL grant di setiap database yang ada) — kalau jadi
  // default, membuat user baru diam-diam memberi akses ke SELURUH database
  // yang sudah ada di server, padahal alur normalnya user dibuat dulu lalu
  // databasenya menyusul. Kosongkan = user baru belum punya akses ke mana
  // pun, lalu diberikan lewat "Assign user" di tab Data.
  const [allDbs, setAllDbs] = useState(false);
  const [selectedDbs, setSelectedDbs] = useState<string[]>([]);
  const [selectedPrivs, setSelectedPrivs] = useState<string[]>(['read', 'write']);
  const [saveCredential, setSaveCredential] = useState(true);

  const [credentials, setCredentials] = useState<website.DBCredentialInfo[]>([]);
  // dbDomains: SEMUA domain yang memakai tiap database di server+engine ini
  // (bukan cuma domain yang sedang dibuka, kalau panel ini dibuka dari tab
  // per-domain) — dipakai kolom "Domain" di daftar database, supaya
  // terlihat juga saat panel dibuka dari modul Database top-level per
  // server yang tidak terikat satu domain.
  const [dbDomains, setDbDomains] = useState<Record<string, string[]>>({});
  // allDomains: semua domain/subdomain YANG ADA di server ini (parent
  // maupun subdomain, satu daftar rata) — sumber pilihan untuk dialog
  // "Kelola domain" dan form buat database, TIDAK bergantung engine
  // (domain website tidak terikat MySQL/PostgreSQL).
  const [allDomains, setAllDomains] = useState<string[]>([]);
  // domainManageTarget: database yang sedang dibuka dialog "Kelola
  // domain"-nya (multi-select, bukan toggle satu domain per klik seperti
  // sebelumnya) — null berarti dialog tertutup.
  const [domainManageTarget, setDomainManageTarget] = useState<string | null>(null);
  const [domainManageSelected, setDomainManageSelected] = useState<Set<string>>(new Set());
  // showAllDatabases: kalau panel ini dibuka dari tab per-domain (domain
  // terisi), daftar SECARA DEFAULT hanya menampilkan database yang sudah
  // tertaut ke domain itu — toggle ini membuka daftar lengkap server saat
  // user perlu menautkan database LAIN yang sudah ada ke domain ini.
  const [showAllDatabases, setShowAllDatabases] = useState(false);
  // newDbDomains: domain yang dipilih saat MEMBUAT database baru, supaya
  // tautannya langsung terbentuk tanpa langkah tambahan sesudahnya.
  // Pra-dicentang ke domain yang sedang dibuka (kalau ada) — membuat
  // database dari dalam tab satu domain wajar diasumsikan untuk domain itu.
  const [newDbDomains, setNewDbDomains] = useState<Set<string>>(new Set(domain ? [domain] : []));
  // linkingUser menunjuk SATU (username, host) spesifik — user MySQL yang
  // terdaftar di beberapa host digabung jadi satu baris di tabel (lihat
  // DBUserInfo.hosts), tapi kredensial/koneksi Explore tetap terikat host
  // TERTENTU, jadi aksi "Hubungkan kredensial" selalu untuk satu host,
  // dipilih lewat baris per-host di kolom aksi tabel.
  const [linkingUser, setLinkingUser] = useState<{ username: string; host: string } | null>(null);
  const [linkPassword, setLinkPassword] = useState('');
  // User yang sedang dibuka dialog "Ubah privilege"-nya.
  const [privilegeTarget, setPrivilegeTarget] = useState<website.DBUserInfo | null>(null);

  // Dua concern yang tadinya campur di satu layar (siapa yang boleh akses
  // vs isi datanya sendiri) sekarang tab terpisah — bukan lagi modal Explore
  // yang muncul di atas semuanya. dataTarget: kredensial mana yang lagi
  // dibuka di tab Data, diisi otomatis (kredensial pertama utk engine ini)
  // atau eksplisit lewat tombol "Explore" di baris user tab sebelah.
  const [panelTab, setPanelTab] = useState<'access' | 'data' | 'settings'>('access');
  const [dataTarget, setDataTarget] = useState<{ username: string; host: string } | null>(null);

  const [dockerAccess, setDockerAccess] = useState<website.DBDockerAccessStatus | null>(null);
  const [dockerAccessBusy, setDockerAccessBusy] = useState(false);

  const [installVersions, setInstallVersions] = useState<string[]>([]);
  const [installVersion, setInstallVersion] = useState('');

  // Guard generasi permintaan — tiap kali (serverId, engine, domain) berubah,
  // load() BARU dimulai dengan id yang lebih besar; sebelum request lama
  // sempat selesai, hasilnya dibuang (bukan ditulis ke state) supaya
  // switch MySQL->PostgreSQL yang cepat tidak pernah menampilkan data
  // engine yang SALAH (bug nyata yang dilaporkan: daftar database/user
  // dari engine sebelumnya sempat tertampil di bawah label engine baru).
  const loadIdRef = useRef(0);

  useEffect(() => {
    GetWebsiteDBPrivileges().then(setPrivileges).catch(() => setPrivileges([]));
  }, []);

  useEffect(() => {
    setInstallVersion('');
    GetWebsiteDBVersions(engine).then(setInstallVersions).catch(() => setInstallVersions([]));
  }, [engine]);

  // load({reset}) — reset TRUE cuma saat konteksnya benar-benar ganti
  // (server/engine/domain lain): isi layar dikosongkan dulu supaya tidak ada
  // sisa data engine sebelumnya yang sempat tampil di bawah label engine
  // baru. Untuk refresh biasa sesudah aksi (buat database, assign user, dst)
  // reset FALSE: data lama tetap tampil sampai data baru datang, cuma
  // indikator `loading` yang menyala — panel tidak berkedip kosong tiap kali
  // selesai satu aksi kecil.
  async function load(opts: { reset?: boolean } = {}) {
    const myLoadId = ++loadIdRef.current;
    setError(null);
    setLoading(true);
    if (opts.reset) {
      setStatus(null);
      setDatabases([]);
      setUsers([]);
      setDockerAccess(null);
    }
    try {
      const st = await GetWebsiteDBStatus(serverId, engine);
      if (loadIdRef.current !== myLoadId) return; // sudah ada load() lebih baru — buang hasil ini
      setStatus(st);
      if (st.installed && st.active) {
        const [dbs, us, da] = await Promise.all([
          ListWebsiteDatabases(serverId, engine),
          ListWebsiteDatabaseUsers(serverId, engine),
          GetWebsiteDBDockerAccessStatus(serverId, engine),
        ]);
        if (loadIdRef.current !== myLoadId) return;
        setDatabases(dbs);
        setUsers(us);
        setDockerAccess(da);
      }
      const creds = await ListWebsiteDBCredentials(serverId);
      if (loadIdRef.current !== myLoadId) return;
      setCredentials(creds);

      // Kolom "Domain" terisi TERLEPAS dari apakah panel ini dibuka dari
      // tab per-domain atau modul top-level per server — dulu domain yang
      // memakai satu database hanya terlihat kalau kebetulan membuka panel
      // dari domain itu sendiri, sekarang selalu terlihat.
      const allLinks = await ListWebsiteServerDatabaseDomains(serverId, engine);
      if (loadIdRef.current !== myLoadId) return;
      const domainMap: Record<string, string[]> = {};
      for (const l of allLinks) {
        (domainMap[l.database] ??= []).push(l.domain);
      }
      setDbDomains(domainMap);
    } catch (e) {
      if (loadIdRef.current !== myLoadId) return;
      setError(String(e));
    } finally {
      if (loadIdRef.current === myLoadId) setLoading(false);
    }
  }

  useEffect(() => {
    void load({ reset: true });
    return () => {
      if (streamIdRef.current) void StopDockerStream(streamIdRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, engine, domain]);

  // Daftar domain/subdomain di server ini — dipakai dialog "Kelola domain"
  // dan form buat database, TIDAK bergantung engine (jadi ambil sekali per
  // server, tidak ikut re-fetch tiap ganti MySQL<->PostgreSQL).
  useEffect(() => {
    ListWebsites(serverId)
      .then((res) => setAllDomains((res.domains ?? []).map((d) => d.domain)))
      .catch(() => setAllDomains([]));
  }, [serverId]);

  function credentialFor(username: string, host: string) {
    const normalizedHost = engine === 'mysql' ? host || '%' : '-';
    return credentials.find((c) => c.engine === engine && c.username === username && c.host === normalizedHost);
  }

  function credentialLabel(username: string, host: string) {
    return engine === 'mysql' ? `${username}@${host || '%'}` : username;
  }

  // Ganti engine (MySQL <-> PostgreSQL) berarti user/kredensialnya beda
  // total — kembali ke tab "Users & Akses", lepas pilihan Explore lama, dan
  // TUTUP semua form yang masih terisi. Form yang dibiarkan hidup lintas
  // engine bikin salah sasaran: user yang diketik (bahkan yang barusan
  // gagal dibuat) di PostgreSQL muncul lagi di tab MySQL seolah-olah mau
  // dibuat di sana.
  useEffect(() => {
    setPanelTab('access');
    setDataTarget(null);
    setShowUserForm(false);
    setNewUser('');
    setNewPassword('');
    setNewHost('%');
    setAllDbs(false);
    setSelectedDbs([]);
    setNewDbName('');
    setNewDbOwner('');
    setAssignTarget(null);
    setAssignUsername('');
    setLinkingUser(null);
    setLinkPassword('');
    setDomainManageTarget(null);
    setNewDbDomains(new Set(domain ? [domain] : []));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [engine]);

  // Membuka HALAMAN explorer untuk user ini (lihat render: dataTarget terisi
  // = seluruh panel diganti halaman explorer).
  function openExplore(username: string, host: string) {
    setDataTarget({ username, host });
  }

  async function handleLinkCredential() {
    if (!linkingUser || !linkPassword.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await SaveWebsiteDBCredential(
        new website.SaveDBCredentialRequest({
          serverId, engine, username: linkingUser.username, host: linkingUser.host || undefined, password: linkPassword, verify: true,
        }),
      );
      setLinkingUser(null);
      setLinkPassword('');
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleForgetCredential(username: string, host: string) {
    const ok = await confirm({
      title: t('db.forgetPassword'),
      message: t('db.forgetPasswordConfirm', { label: credentialLabel(username, host) }),
      detail: t('db.forgetPasswordDetail'),
      confirmLabel: t('db.forgetPassword'),
    });
    if (!ok) return;
    setBusy(true);
    try {
      await ForgetWebsiteDBCredential(serverId, engine, username, host);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  // Membuka dialog "Kelola domain" — multi-select, pra-dicentang ke domain
  // yang SUDAH tertaut ke database ini (dari dbDomains, sudah mencakup
  // SEMUA domain, tidak cuma domain yang sedang dibuka).
  function openDomainManage(dbName: string) {
    setDomainManageTarget(dbName);
    setDomainManageSelected(new Set(dbDomains[dbName] ?? []));
  }

  async function handleSaveDomainManage() {
    if (!domainManageTarget) return;
    setBusy(true);
    setError(null);
    try {
      await SetWebsiteDatabaseDomains(serverId, engine, domainManageTarget, Array.from(domainManageSelected));
      setDomainManageTarget(null);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  function toggleDomainManageSelection(dom: string) {
    setDomainManageSelected((prev) => {
      const next = new Set(prev);
      if (next.has(dom)) next.delete(dom);
      else next.add(dom);
      return next;
    });
  }

  async function handleStart() {
    setBusy(true);
    setError(null);
    try {
      await StartWebsiteDB(serverId, engine);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleToggleDockerAccess(enable: boolean) {
    if (!enable) {
      const ok = await confirm({
        title: t('db.dockerDisable'),
        message: t('db.dockerDisableConfirm', { engine: engineLabel }),
        detail: t('db.dockerDisableDetail'),
        confirmLabel: t('db.dockerDisable'),
        danger: true,
      });
      if (!ok) return;
    }
    setDockerAccessBusy(true);
    setError(null);
    try {
      setDockerAccess(
        enable ? await EnsureWebsiteDBDockerAccess(serverId, engine) : await DisableWebsiteDBDockerAccess(serverId, engine),
      );
    } catch (e) {
      setError(String(e));
    } finally {
      setDockerAccessBusy(false);
    }
  }

  async function handleInstall() {
    setLines([]);
    setError(null);
    setInstalling(true);
    const streamId = await StreamWebsiteInstall(serverId, engine, installVersion);
    streamIdRef.current = streamId;
    const unsub = EventsOn(`website:install:${streamId}`, (evt: StreamLineEvent) => {
      if (evt.type === 'line' && evt.line) setLines((l) => [...l, evt.line as string]);
      else if (evt.type === 'error') {
        setError(evt.message ?? 'Instalasi gagal');
        setInstalling(false);
        streamIdRef.current = null;
        unsub();
      } else if (evt.type === 'end') {
        setInstalling(false);
        streamIdRef.current = null;
        unsub();
        void load();
      }
    });
  }

  async function handleCreateDb() {
    if (!newDbName.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const createdName = newDbName.trim();
      await CreateWebsiteDatabase(
        new website.DBCreateDatabaseRequest({
          serverId,
          engine,
          name: createdName,
          owner: newDbOwner || undefined,
        }),
      );
      if (newDbDomains.size > 0) {
        await SetWebsiteDatabaseDomains(serverId, engine, createdName, Array.from(newDbDomains));
      }
      setNewDbName('');
      setNewDbOwner('');
      setNewDbDomains(new Set(domain ? [domain] : []));
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  // usersForDatabase membalik daftar user->databases jadi database->users,
  // jadi panel "assign user" tidak perlu endpoint baru: datanya sudah ada
  // di hasil ListWebsiteDatabaseUsers yang sama.
  function usersForDatabase(dbName: string) {
    return users.filter((u) => u.allDatabases || (u.databases ?? []).includes(dbName));
  }

  async function handleAssignUser(dbName: string, username: string) {
    if (!username) return;
    setBusy(true);
    setError(null);
    try {
      await GrantWebsiteDatabaseUser(
        new website.DBDatabaseUserRequest({ serverId, engine, database: dbName, username, privileges: selectedPrivs }),
      );
      setAssignUsername('');
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  // Dua konfirmasi untuk hapus database: aksinya permanen dan tidak ada
  // undo sama sekali di sisi server (bukan soft delete).
  async function handleDropDatabase(dbName: string) {
    const ok = await confirm({
      title: t('db.dropDatabase'),
      message: t('db.dropDatabaseConfirm', { db: dbName }),
      detail: t('db.dropDatabaseDetail'),
      confirmLabel: t('common.delete'),
      danger: true,
    });
    if (!ok) return;
    const confirmedTwice = await confirm({
      title: t('common.confirm'),
      message: t('db.dropDatabaseAgain', { db: dbName }),
      confirmLabel: t('db.dropDatabaseAgainConfirm'),
      danger: true,
    });
    if (!confirmedTwice) return;
    setBusy(true);
    setError(null);
    try {
      await DropWebsiteDatabase(serverId, engine, dbName);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleDropUser(user: website.DBUserInfo) {
    const label = engine === 'mysql' && user.host ? `${user.username} (${user.host})` : user.username;
    const ok = await confirm({
      title: t('db.dropUser'),
      message: t('db.dropUserConfirm', { label }),
      detail: engine === 'postgresql' ? t('db.dropUserDetailPG') : t('db.dropUserDetailMySQL'),
      confirmLabel: t('db.dropUser'),
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    setError(null);
    try {
      await DropWebsiteDatabaseUser(serverId, engine, user.username, '');
      if (dataTarget?.username === user.username) setDataTarget(null);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleRevokeUser(dbName: string, username: string) {
    const ok = await confirm({
      title: t('db.revoke'),
      message: t('db.revokeConfirm', { user: username, db: dbName }),
      confirmLabel: t('db.revoke'),
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    setError(null);
    try {
      await RevokeWebsiteDatabaseUser(new website.DBDatabaseUserRequest({ serverId, engine, database: dbName, username }));
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleCreateUser() {
    if (!newUser.trim() || !newPassword.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await CreateWebsiteDatabaseUser(
        new website.DBCreateUserRequest({
          serverId,
          engine,
          username: newUser.trim(),
          password: newPassword,
          host: engine === 'mysql' ? newHost.trim() || '%' : undefined,
          allDbs: allDbs,
          databases: allDbs ? [] : selectedDbs,
          privileges: selectedPrivs,
          saveCredential,
        }),
      );
      setNewUser('');
      setNewPassword('');
      setShowUserForm(false);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  // Menyetel privilege user ke persis daftar yang dipilih di dialog "Ubah
  // privilege" — cakupan database TIDAK ikut berubah (itu urusan "Assign
  // user"/tombol ×). User MySQL yang terdaftar di beberapa host digabung
  // jadi satu baris, jadi diterapkan ke SEMUA host-nya satu-satu: grants
  // MySQL memang per-host. PostgreSQL tidak punya konsep host sama sekali.
  async function handleApplyPrivileges(user: website.DBUserInfo, privileges: string[]) {
    setBusy(true);
    setError(null);
    try {
      const hosts = engine === 'mysql' ? (user.hosts && user.hosts.length > 0 ? user.hosts : [user.host ?? '%']) : [undefined];
      for (const host of hosts) {
        await SetWebsiteDatabaseGrants(
          new website.DBGrantsRequest({
            serverId,
            engine,
            username: user.username,
            host,
            allDbs: !!user.allDatabases,
            databases: user.allDatabases ? [] : (user.databases ?? []),
            privileges,
          }),
        );
      }
      setPrivilegeTarget(null);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  function togglePriv(key: string) {
    setSelectedPrivs((p) => (p.includes(key) ? p.filter((x) => x !== key) : [...p, key]));
  }

  function toggleDb(name: string) {
    setSelectedDbs((d) => (d.includes(name) ? d.filter((x) => x !== name) : [...d, name]));
  }

  const engineLabel = engine === 'mysql' ? 'MySQL / MariaDB' : 'PostgreSQL';

  // Dibuka dari tab per-domain (domain terisi): daftar SECARA DEFAULT
  // hanya database yang sudah tertaut ke domain itu — dulu SELALU semua
  // database di server tampil, tidak peduli domain yang sedang dibuka.
  // showAllDatabases membuka daftar lengkap saat perlu menautkan database
  // lain yang sudah ada.
  const visibleDatabases =
    domain && !showAllDatabases ? databases.filter((d) => (dbDomains[d.name] ?? []).includes(domain)) : databases;

  // installVersions untuk engine "mysql" berprefix produk ("mariadb-10.11")
  // supaya nanti bisa berdampingan dengan "mysql-*" (MySQL asli, belum ada)
  // di satu dropdown yang sama — cuma label tampilannya yang dirapikan di sini.
  function versionLabel(v: string) {
    if (v.startsWith('mariadb-')) return `MariaDB ${v.slice('mariadb-'.length)}`;
    if (v.startsWith('mysql-')) return `MySQL ${v.slice('mysql-'.length)}`;
    return v;
  }

  // Explorer punya HALAMAN sendiri: begitu satu user dibuka lewat aksi
  // "Explore", seluruh panel Database diganti oleh halaman explorer itu
  // (bukan disisipkan di bawah daftar database seperti sebelumnya, yang
  // bikin satu layar mengerjakan dua hal sekaligus dan sempit). Pola yang
  // sama dipakai halaman fitur domain di Website.
  if (dataTarget) {
    return (
      <div className="db-explorer-page">
        <div className="db-explorer-page__header">
          <button className="btn btn--ghost btn--sm" onClick={() => setDataTarget(null)}>
            <ArrowLeft size={14} /> {t('common.back')}
          </button>
          <h3 style={{ margin: 0, fontSize: 14 }}>
            Explore — {credentialLabel(dataTarget.username, dataTarget.host)}
          </h3>
          <span className="docker-badge" style={{ marginLeft: 'auto' }}>
            {engine === 'mysql' ? 'MySQL / MariaDB' : 'PostgreSQL'}
          </span>
        </div>
        {engine === 'mysql' ? (
          <MySQLExplorer
            key={`${dataTarget.username}@${dataTarget.host}`}
            serverId={serverId}
            username={dataTarget.username}
            host={dataTarget.host}
          />
        ) : (
          <PGExplorer key={dataTarget.username} serverId={serverId} username={dataTarget.username} />
        )}
      </div>
    );
  }

  return (
    <div>
      <div className="segmented" style={{ marginBottom: 10 }}>
        <button className={`segmented__item${engine === 'mysql' ? ' segmented__item--active' : ''}`} onClick={() => setEngine('mysql')}>
          MySQL / MariaDB
        </button>
        <button
          className={`segmented__item${engine === 'postgresql' ? ' segmented__item--active' : ''}`}
          onClick={() => setEngine('postgresql')}
        >
          PostgreSQL
        </button>
      </div>

      {error && <p className="overview__error">{error}</p>}
      {/* Dua tingkat indikator: layar "Memuat…" penuh cuma saat belum ada
          data sama sekali (ganti engine/server), dan strip tipis di atas
          konten untuk refresh biasa supaya data lama tetap kebaca. */}
      {!status && loading && <p className="workspace__placeholder">{t('common.loading')}</p>}
      {status && (loading || busy) && (
        <p className="chmod-path" style={{ marginTop: 0 }}>
          <span className="spinner" /> {busy ? t('common.processing') : t('common.loading')}
        </p>
      )}

      {status && status.installed && !status.active && (
        <div className="docker-engine">
          <p>{t('db.notActive', { engine: engineLabel })}</p>
          <button className="btn btn--primary" disabled={busy} onClick={() => void handleStart()}>
            {busy && <span className="spinner" />} {t('db.start')}
          </button>
        </div>
      )}

      {status && !status.installed && (
        <div className="docker-engine">
          <p>
            {t('db.notInstalled', { engine: engineLabel, distro: status.distroName ? ` (distro: ${status.distroName})` : '' })}
          </p>
          {!status.canInstall && <p className="overview__error">{t('db.distroUnsupported')}</p>}
          {status.canInstall && (
            <div className="docker-recreate__row">
              <select value={installVersion} onChange={(e) => setInstallVersion(e.target.value)} disabled={installing}>
                <option value="">{t('db.defaultVersion')}</option>
                {installVersions.map((v) => (
                  <option key={v} value={v}>
                    {versionLabel(v)}
                  </option>
                ))}
              </select>
              <button className="btn btn--primary" disabled={installing} onClick={() => void handleInstall()}>
                {installing && <span className="spinner" />} {installing ? t('db.installing') : t('db.install', { engine: engine === 'mysql' ? 'MariaDB' : 'PostgreSQL' })}
              </button>
            </div>
          )}
          {installVersion && (
            <p className="chmod-path">
              Versi {versionLabel(installVersion)} dipasang lewat repo resmi {engine === 'mysql' ? 'MariaDB' : 'PGDG'} — repo ditambahkan otomatis,
              tidak perlu langkah manual.
            </p>
          )}
          {lines.length > 0 && <pre className="docker-engine__log">{lines.join('\n')}</pre>}
        </div>
      )}

      {status && status.installed && status.active && (
        <>
          {status.version && (
            <p className="chmod-path" style={{ marginTop: 0 }}>
              {status.version}
              {status.repoConfigured && ' · terpasang lewat repo resmi vendor (versi pinned)'}
            </p>
          )}

          <div className="segmented" style={{ marginBottom: 14 }}>
            <button className={`segmented__item${panelTab === 'access' ? ' segmented__item--active' : ''}`} onClick={() => setPanelTab('access')}>
              {t('db.tab.access')}
            </button>
            <button className={`segmented__item${panelTab === 'data' ? ' segmented__item--active' : ''}`} onClick={() => setPanelTab('data')}>
              {t('db.tab.data')}
            </button>
            <button className={`segmented__item${panelTab === 'settings' ? ' segmented__item--active' : ''}`} onClick={() => setPanelTab('settings')}>
              {t('db.tab.settings')}
            </button>
          </div>

          {/* Pengaturan per-engine — MySQL dan PostgreSQL punya halaman
              sendiri karena panel ini memang sudah ter-scope ke satu engine
              (segmented di atas), jadi apa pun di sini hanya berlaku untuk
              engine yang sedang dipilih. */}
          {panelTab === 'settings' && (
            <section>
              <div className="files-panel__toolbar">
                <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: 0 }}>
                  {t('db.settingsTitle', { engine: engineLabel })}
                </h3>
              </div>

              {!dockerAccess ? (
                <p className="workspace__placeholder">{t('common.loading')}</p>
              ) : (
                <div className="docker-engine">
                  <div className="files-panel__toolbar" style={{ marginBottom: 6 }}>
                    <strong style={{ fontSize: 13 }}>{t('db.dockerAccess')}</strong>
                    <span
                      className={`docker-badge ${dockerAccess.enabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}
                      style={{ marginLeft: 8 }}
                    >
                      {dockerAccess.enabled ? t('common.active') : t('common.inactive')}
                    </span>
                  </div>

                  <p style={{ marginTop: 0 }}>
                    {dockerAccess.enabled ? (
                      <>
                        <CircleCheck size={13} /> {t('db.dockerAccessOn')}
                      </>
                    ) : (
                      <>
                        <TriangleAlert size={13} />{' '}
                        {dockerAccess.bindAllInterfaces ? t('db.dockerAccessOffFirewall') : t('db.dockerAccessOffBind')}
                      </>
                    )}
                  </p>

                  <p className="chmod-path" style={{ marginTop: 0 }}>
                    {t('db.dockerListenAll')}: <strong>{dockerAccess.bindAllInterfaces ? t('common.yes') : t('common.no')}</strong> ·{' '}
                    {t('db.dockerFirewall')}:{' '}
                    <strong>{!dockerAccess.firewallDetected || dockerAccess.firewallDetected === 'none' ? t('common.none') : dockerAccess.firewallDetected}</strong>
                    {dockerAccess.firewallDetected && dockerAccess.firewallDetected !== 'none' && (
                      <> ({dockerAccess.firewallRuleActive ? t('db.dockerRuleInstalled') : t('db.dockerRuleMissing')})</>
                    )}
                  </p>

                  {dockerAccess.message && <p className="overview__error">{dockerAccess.message}</p>}

                  {dockerAccess.enabled ? (
                    <button className="btn btn--sm btn--danger" disabled={dockerAccessBusy} onClick={() => void handleToggleDockerAccess(false)}>
                      {dockerAccessBusy ? (<><span className="spinner" /> {t('db.applying')}</>) : (<><Boxes size={13} /> {t('db.dockerDisable')}</>)}
                    </button>
                  ) : (
                    <button className="btn btn--sm btn--primary" disabled={dockerAccessBusy} onClick={() => void handleToggleDockerAccess(true)}>
                      {dockerAccessBusy ? (<><span className="spinner" /> {t('db.applying')}</>) : (<><Boxes size={13} /> {t('db.dockerEnable')}</>)}
                    </button>
                  )}
                </div>
              )}
            </section>
          )}

          {panelTab === 'access' && (
          <>
          <section>
            <div className="files-panel__toolbar">
              <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: 0 }}>{t('db.section.users')}</h3>
              <button className="btn btn--sm btn--primary" style={{ marginLeft: 'auto' }} onClick={() => setShowUserForm((v) => !v)}>
                {t('db.newUser')}
              </button>
            </div>

            {showUserForm && (
              <div className="docker-recreate">
                <div className="docker-recreate__row">
                  <input placeholder="username" value={newUser} onChange={(e) => setNewUser(e.target.value)} />
                  <input type="password" placeholder="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
                  {engine === 'mysql' && <input placeholder="host (%)" value={newHost} onChange={(e) => setNewHost(e.target.value)} style={{ maxWidth: 100 }} />}
                </div>
                <label className="form-check">
                  <input type="checkbox" checked={allDbs} onChange={(e) => setAllDbs(e.target.checked)} />
                  <span>{t('db.accessAllDatabases')}</span>
                </label>
                {allDbs && (
                  <p className="chmod-path" style={{ margin: '4px 0 0' }}>
                    <TriangleAlert size={12} /> {t('db.accessAllWarning', { count: databases.length })}
                  </p>
                )}
                {!allDbs && (
                  <div className="chip-row" style={{ display: 'flex', gap: 6, flexWrap: 'wrap', margin: '6px 0' }}>
                    {databases.map((d) => (
                      <label key={d.name} className="form-check" style={{ marginTop: 0 }}>
                        <input type="checkbox" checked={selectedDbs.includes(d.name)} onChange={() => toggleDb(d.name)} />
                        <span>{d.name}</span>
                      </label>
                    ))}
                    {databases.length === 0 && (
                      <span className="chmod-path">{t('db.noDatabasesOnServer')}</span>
                    )}
                  </div>
                )}
                {!allDbs && selectedDbs.length === 0 && (
                  <p className="chmod-path" style={{ margin: '4px 0 0' }}>
                    {t('db.noDatabaseSelected')}
                  </p>
                )}
                <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', margin: '8px 0' }}>
                  {privileges.map((p) => (
                    <label key={p.key} className="form-check" style={{ marginTop: 0 }}>
                      <input type="checkbox" checked={selectedPrivs.includes(p.key)} onChange={() => togglePriv(p.key)} />
                      <span>{p.label}</span>
                    </label>
                  ))}
                </div>
                <label className="form-check">
                  <input type="checkbox" checked={saveCredential} onChange={(e) => setSaveCredential(e.target.checked)} />
                  <span>{t('db.saveCredential')}</span>
                </label>
                <button className="btn btn--sm btn--primary" disabled={busy || !newUser.trim() || !newPassword.trim()} onClick={() => void handleCreateUser()}>
                  {busy && <span className="spinner" />} {t('db.createUser')}
                </button>
              </div>
            )}

            <div className="db-table-scroll">
              <table className="files-panel__table">
                <thead>
                  <tr>
                    <th>{t('common.username')}</th>
                    {engine === 'mysql' && <th>{t('common.host')}</th>}
                    <th>{t('db.colDatabase')}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((u) => {
                    // User MySQL yang sama di beberapa host digabung jadi satu
                    // baris (lihat DBUserInfo.hosts). Kredensial/Explore tetap
                    // terikat host TERTENTU, jadi di dalam menu aksi tiap host
                    // punya entri sendiri — bukan lagi tombol bertumpuk yang
                    // bikin baris tabel jadi tinggi.
                    const hosts = engine === 'mysql' ? (u.hosts && u.hosts.length > 0 ? u.hosts : [u.host ?? '%']) : [''];
                    const dbAccessLabel = u.allDatabases
                      ? t('db.allDatabases')
                      : u.databases && u.databases.length > 0
                        ? u.databases.join(', ')
                        : '—';
                    return (
                      <tr key={u.username}>
                        <td>{u.username}</td>
                        {engine === 'mysql' && <td>{u.host}</td>}
                        <td title={dbAccessLabel} style={{ maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {dbAccessLabel}
                        </td>
                        <td className="files-panel__row-actions">
                          <ActionMenu title={`${t('common.actions')} — ${u.username}`}>
                            {(close) => (
                              <>
                                {hosts.map((h) => {
                                  const cred = credentialFor(u.username, h);
                                  return (
                                    <div key={h || '-'}>
                                      {engine === 'mysql' && hosts.length > 1 && <div className="action-menu__label">{h}</div>}
                                      <button
                                        disabled={busy}
                                        onClick={() => {
                                          close();
                                          // Explore selalu tersedia per user:
                                          // kalau password-nya belum tersimpan,
                                          // langsung buka form hubungkan dulu
                                          // (bukan menyembunyikan menunya).
                                          if (cred) openExplore(u.username, h);
                                          else setLinkingUser({ username: u.username, host: h });
                                        }}
                                      >
                                        <Search size={13} /> {cred ? t('db.explore') : t('db.exploreLinkFirst')}
                                      </button>
                                      {cred && (
                                        <button
                                          disabled={busy}
                                          onClick={() => {
                                            close();
                                            void handleForgetCredential(u.username, h);
                                          }}
                                        >
                                          <X size={13} /> {t('db.forgetPassword')}
                                        </button>
                                      )}
                                    </div>
                                  );
                                })}
                                <div className="action-menu__sep" />
                                <button
                                  disabled={busy}
                                  onClick={() => {
                                    close();
                                    setPrivilegeTarget(u);
                                  }}
                                >
                                  <KeyRound size={13} /> {t('db.changePrivileges')}
                                </button>
                                <button
                                  className="action-menu__danger"
                                  disabled={busy}
                                  onClick={() => {
                                    close();
                                    void handleDropUser(u);
                                  }}
                                >
                                  <Trash2 size={13} /> {t('db.dropUser')}
                                </button>
                              </>
                            )}
                          </ActionMenu>
                        </td>
                      </tr>
                    );
                  })}
                  {users.length === 0 && (
                    <tr>
                      <td colSpan={4} className="files-panel__empty">
                        {t('db.noUsers')}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

          </section>
          </>
          )}

          {panelTab === 'data' && (
            <>
            <section style={{ marginBottom: 16 }}>
              <div className="files-panel__toolbar">
                <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: 0 }}>{t('db.section.databases')}</h3>
                <input placeholder={t('db.newDbName')} value={newDbName} onChange={(e) => setNewDbName(e.target.value)} style={{ marginLeft: 'auto' }} />
                <select value={newDbOwner} onChange={(e) => setNewDbOwner(e.target.value)} title="User pemilik database baru ini">
                  <option value="">{t('db.noOwner')}</option>
                  {users.map((u) => (
                    <option key={u.username} value={u.username}>
                      {u.username}
                    </option>
                  ))}
                </select>
                <button className="btn btn--sm btn--primary" disabled={busy || !newDbName.trim()} onClick={() => void handleCreateDb()}>
                  {busy && <span className="spinner" />} {t('db.create')}
                </button>
              </div>
              {allDomains.length > 0 && (
                <div className="chip-row" style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', margin: '0 0 8px' }}>
                  <span className="chmod-path" style={{ margin: 0 }}>
                    {t('db.linkOnCreate')}
                  </span>
                  {allDomains.map((dom) => (
                    <label key={dom} className="form-check" style={{ marginTop: 0 }}>
                      <input
                        type="checkbox"
                        checked={newDbDomains.has(dom)}
                        onChange={() =>
                          setNewDbDomains((prev) => {
                            const next = new Set(prev);
                            if (next.has(dom)) next.delete(dom);
                            else next.add(dom);
                            return next;
                          })
                        }
                      />
                      <span>{dom}</span>
                    </label>
                  ))}
                </div>
              )}
              {domain && (
                <label className="form-check" style={{ marginBottom: 8 }}>
                  <input type="checkbox" checked={showAllDatabases} onChange={(e) => setShowAllDatabases(e.target.checked)} />
                  <span>{t('db.showAllDatabases')}</span>
                </label>
              )}
              <div className="db-table-scroll">
                <table className="files-panel__table">
                  <thead>
                    <tr>
                      <th>{t('common.name')}</th>
                      <th>{t('db.colDomain')}</th>
                      <th>{t('db.colUsersWithAccess')}</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {visibleDatabases.map((d) => {
                      const dbUsers = usersForDatabase(d.name);
                      const linkedDomains = dbDomains[d.name] ?? [];
                      return (
                        <tr key={d.name}>
                          <td>{d.name}</td>
                          <td>
                            {linkedDomains.length === 0 ? (
                              <span style={{ opacity: 0.5 }}>—</span>
                            ) : (
                              <span style={{ display: 'inline-flex', flexWrap: 'wrap', gap: 4 }}>
                                {linkedDomains.map((dom) => (
                                  <span key={dom} className="tag-chip tag-chip--readonly">
                                    {dom}
                                  </span>
                                ))}
                              </span>
                            )}
                          </td>
                          <td>
                            {dbUsers.length === 0 ? (
                              <span style={{ opacity: 0.5 }}>—</span>
                            ) : (
                              <span style={{ display: 'inline-flex', flexWrap: 'wrap', gap: 4 }}>
                                {dbUsers.map((u) => (
                                  <span key={u.username} className="tag-chip tag-chip--readonly">
                                    {u.username}
                                    {u.allDatabases && <span style={{ opacity: 0.5 }}> (global)</span>}
                                    {!u.allDatabases && (
                                      <button
                                        title={t('db.revoke')}
                                        disabled={busy}
                                        onClick={() => void handleRevokeUser(d.name, u.username)}
                                      >
                                        <X size={11} />
                                      </button>
                                    )}
                                  </span>
                                ))}
                              </span>
                            )}
                          </td>
                          <td className="files-panel__row-actions">
                            <ActionMenu title={`${t('common.actions')} — ${d.name}`}>
                              {(close) => (
                                <>
                                  <button
                                    disabled={busy}
                                    onClick={() => {
                                      close();
                                      setAssignTarget(assignTarget === d.name ? null : d.name);
                                    }}
                                  >
                                    <UserPlus size={13} /> {t('db.assignUser')}
                                  </button>
                                  <button
                                    disabled={busy}
                                    onClick={() => {
                                      close();
                                      openDomainManage(d.name);
                                    }}
                                  >
                                    <Link2 size={13} /> {t('db.manageDomains')}
                                  </button>
                                  <div className="action-menu__sep" />
                                  <button
                                    className="action-menu__danger"
                                    disabled={busy}
                                    onClick={() => {
                                      close();
                                      void handleDropDatabase(d.name);
                                    }}
                                  >
                                    <Trash2 size={13} /> {t('db.dropDatabase')}
                                  </button>
                                </>
                              )}
                            </ActionMenu>
                          </td>
                        </tr>
                      );
                    })}
                    {visibleDatabases.length === 0 && (
                      <tr>
                        <td colSpan={4} className="files-panel__empty">
                          {databases.length === 0
                            ? t('db.noDatabases')
                            : t('db.noDatabasesLinkedToDomain')}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>

              {assignTarget && (
                <div className="docker-recreate">
                  <p>
                    {t('db.assignDescription', { db: assignTarget })} ({selectedPrivs.join(', ') || 'read'})
                  </p>
                  <div className="docker-recreate__row">
                    <select value={assignUsername} onChange={(e) => setAssignUsername(e.target.value)}>
                      <option value="">{t('db.assignPick')}</option>
                      {users
                        .filter((u) => !usersForDatabase(assignTarget).some((x) => x.username === u.username))
                        .map((u) => (
                          <option key={u.username} value={u.username}>
                            {u.username}
                          </option>
                        ))}
                    </select>
                    <button
                      className="btn btn--sm btn--primary"
                      disabled={busy || !assignUsername}
                      onClick={() => void handleAssignUser(assignTarget, assignUsername)}
                    >
                      {busy && <span className="spinner" />} {t('db.assignGrant')}
                    </button>
                    <button className="btn btn--sm btn--ghost" onClick={() => setAssignTarget(null)}>
                      {t('common.close')}
                    </button>
                  </div>
                </div>
              )}

            </section>

            </>
          )}
        </>
      )}

      {/* Dialog dirender di akar panel (bukan di dalam salah satu tab)
          supaya tetap muncul di atas apa pun yang sedang aktif. */}
      {privilegeTarget && (
        <UserPrivilegesModal
          user={privilegeTarget}
          engine={engine}
          options={privileges}
          busy={busy}
          onClose={() => setPrivilegeTarget(null)}
          onApply={(privs) => void handleApplyPrivileges(privilegeTarget, privs)}
        />
      )}

      {linkingUser && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && setLinkingUser(null)}>
          <div className="modal-card modal-card--small">
            <div className="modal-card__header">
              <h2>{t('db.linkCredential')}</h2>
              <button className="modal-card__close" onClick={() => setLinkingUser(null)}>
                <X size={18} />
              </button>
            </div>
            <div className="modal-card__body">
              <p style={{ margin: 0 }}>
                {t('db.linkCredentialPrompt', { label: credentialLabel(linkingUser.username, linkingUser.host) })}
              </p>
              <label className="form-field">
                <span>{t('common.password')}</span>
                <input
                  type="password"
                  autoFocus
                  placeholder="password"
                  value={linkPassword}
                  onChange={(e) => setLinkPassword(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && linkPassword.trim() && !busy) void handleLinkCredential();
                  }}
                />
              </label>
              <p className="chmod-path" style={{ margin: 0 }}>
                {t('db.linkCredentialHint')}
              </p>
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => setLinkingUser(null)}>
                {t('common.cancel')}
              </button>
              <button className="btn btn--primary" disabled={busy || !linkPassword.trim()} onClick={() => void handleLinkCredential()}>
                {busy && <span className="spinner" />} {t('db.verifyAndSave')}
              </button>
            </div>
          </div>
        </div>
      )}

      {domainManageTarget && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && setDomainManageTarget(null)}>
          <div className="modal-card modal-card--small">
            <div className="modal-card__header">
              <h2>{t('db.manageDomainsTitle', { db: domainManageTarget })}</h2>
              <button className="modal-card__close" onClick={() => setDomainManageTarget(null)}>
                <X size={18} />
              </button>
            </div>
            <div className="modal-card__body">
              <p style={{ margin: 0 }}>{t('db.manageDomainsHint')}</p>
              {allDomains.length === 0 ? (
                <p className="chmod-path" style={{ margin: 0 }}>
                  {t('db.noDomainsOnServer')}
                </p>
              ) : (
                <div className="chip-row" style={{ display: 'flex', flexDirection: 'column', gap: 6, margin: '8px 0' }}>
                  {allDomains.map((dom) => (
                    <label key={dom} className="form-check" style={{ marginTop: 0 }}>
                      <input
                        type="checkbox"
                        checked={domainManageSelected.has(dom)}
                        onChange={() => toggleDomainManageSelection(dom)}
                      />
                      <span>{dom}</span>
                    </label>
                  ))}
                </div>
              )}
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => setDomainManageTarget(null)}>
                {t('common.cancel')}
              </button>
              <button className="btn btn--primary" disabled={busy} onClick={() => void handleSaveDomainManage()}>
                {busy && <span className="spinner" />} {t('common.save')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
