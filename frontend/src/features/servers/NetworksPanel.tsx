import { useEffect, useState } from 'react';
import { ListDockerNetworks, CreateDockerNetwork, RemoveDockerNetwork } from '../../../wailsjs/go/main/App';
import { docker } from '../../../wailsjs/go/models';

const emptyForm = { name: '', driver: 'bridge', subnet: '', gateway: '', internal: false, attachable: false };

export function NetworksPanel({ serverId }: { serverId: string }) {
  const [networks, setNetworks] = useState<docker.NetworkInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [busy, setBusy] = useState(false);

  async function load() {
    setLoading(true);
    setError(null);
    try {
      const res = await ListDockerNetworks(serverId);
      setNetworks(res.networks);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  async function handleCreate() {
    setBusy(true);
    setError(null);
    try {
      await CreateDockerNetwork(
        new docker.NetworkCreateRequest({
          serverId,
          name: form.name,
          driver: form.driver,
          subnet: form.subnet,
          gateway: form.gateway,
          internal: form.internal,
          attachable: form.attachable,
        }),
      );
      setForm(emptyForm);
      setShowForm(false);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleRemove(name: string) {
    if (!confirm(`Hapus network "${name}"?`)) return;
    setBusy(true);
    setError(null);
    try {
      await RemoveDockerNetwork(serverId, name);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="files-panel">
      <div className="files-panel__toolbar">
        <button className="btn btn--sm" disabled={loading} onClick={() => void load()}>
          ⟲ Refresh
        </button>
        <button className="btn btn--sm btn--primary" onClick={() => setShowForm((v) => !v)}>
          + Network
        </button>
      </div>

      {error && <p className="overview__error">{error}</p>}

      {showForm && (
        <div className="docker-recreate__row docker-network-form">
          <input placeholder="nama" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <select value={form.driver} onChange={(e) => setForm({ ...form, driver: e.target.value })}>
            <option value="bridge">bridge</option>
            <option value="overlay">overlay</option>
            <option value="macvlan">macvlan</option>
          </select>
          <input placeholder="subnet (opsional)" value={form.subnet} onChange={(e) => setForm({ ...form, subnet: e.target.value })} />
          <input placeholder="gateway (opsional)" value={form.gateway} onChange={(e) => setForm({ ...form, gateway: e.target.value })} />
          <label>
            <input type="checkbox" checked={form.internal} onChange={(e) => setForm({ ...form, internal: e.target.checked })} />
            internal
          </label>
          <label>
            <input type="checkbox" checked={form.attachable} onChange={(e) => setForm({ ...form, attachable: e.target.checked })} />
            attachable
          </label>
          <button className="btn btn--sm btn--primary" disabled={busy || !form.name.trim()} onClick={() => void handleCreate()}>
            Buat
          </button>
        </div>
      )}

      <div className="files-panel__table-wrap">
        <table className="files-panel__table">
          <thead>
            <tr>
              <th>Nama</th>
              <th>Driver</th>
              <th>Subnet</th>
              <th>Container</th>
              <th className="files-panel__col-actions" />
            </tr>
          </thead>
          <tbody>
            {networks.map((n) => (
              <tr key={n.id}>
                <td>{n.name}</td>
                <td>{n.driver}</td>
                <td>{n.subnet || '—'}</td>
                <td>{n.containerCount}</td>
                <td className="files-panel__row-actions">
                  {!n.builtin && (
                    <button title="Hapus" disabled={busy} onClick={() => void handleRemove(n.name)}>
                      🗑
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {!loading && networks.length === 0 && (
              <tr>
                <td colSpan={5} className="files-panel__empty">
                  Tidak ada network.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {loading && <div className="files-panel__loading">Memuat…</div>}
      </div>
    </div>
  );
}
