import { sha256 } from '@noble/hashes/sha2.js';
import { MobileError, type Schemas } from './session';
import type { NativeUploadPort, NativeUploadSource } from './upload-transfer';

export const MINI_UPLOAD_BYTES = 20 * 1024 * 1024;
type Picked = { path: string; name: string; size: number; media: string };
type Put = (url: string, headers: Record<string, string>, data: ArrayBuffer) => { promise: Promise<{ status: number; etag: string }>; abort(): void };
export type MiniFiles = { pick(): Promise<Picked>; read(path: string): Promise<ArrayBuffer>; size(path: string): Promise<number>; put: Put; key(): string };
type Entry = { source: NativeUploadSource; path: string; upload?: Schemas['Upload']; run?: string };
const hex = (v: Uint8Array) => Array.from(v, n => n.toString(16).padStart(2, '0')).join('');
const fail = (message = '原文件或当前登录已改变，请重新选择并核对上传队列。'): never => { throw new MobileError('MINI_UPLOAD_CHANGED', message); };
const uuid = (key: string) => { const raw = Uint8Array.from(key, c => c.charCodeAt(0)); const h = hex(sha256(raw)).slice(0, 32); return h.slice(0, 8) + '-' + h.slice(8, 12) + '-' + h.slice(12, 16) + '-' + h.slice(16, 20) + '-' + h.slice(20); };

// Only temporary picker references and operation identities live in memory.
// Restart/login loss requires reselecting the exact original; cloud state stays authoritative.
export class MiniUploadSources implements NativeUploadPort {
  private owner = ''; private epoch = 0; private entries = new Map<string, Entry>(); private abort?: () => void;
  constructor(private files: MiniFiles) {}
  clear() { this.pause(); this.owner = ''; this.entries.clear(); this.epoch++; }
  async scope(owner: string) { if (owner !== this.owner) { this.clear(); this.owner = owner; } }
  async list() { return Array.from(this.entries.values(), e => ({ ...e.source })); }
  private entry(handle: string) { if (!this.owner) return fail(); const entry = this.entries.get(handle); if (!entry) return fail(); return entry; }
  private async fingerprint(picked: Picked, epoch: number) {
    if (!Number.isSafeInteger(picked.size) || picked.size < 1 || picked.size > MINI_UPLOAD_BYTES) fail('小程序普通附件需为1字节至20 MiB；大文件请使用 Web／Android。');
    if (await this.files.size(picked.path) !== picked.size || epoch !== this.epoch) fail();
    const raw = new Uint8Array(await this.files.read(picked.path));
    try { if (raw.length !== picked.size || epoch !== this.epoch) fail(); return 'source-sha256-v2:' + hex(sha256(raw)); }
    finally { raw.fill(0); }
  }
  async select(replacement?: string) {
    if (!this.owner) fail(); const epoch = this.epoch;
    const picked = await this.files.pick(), fingerprint = await this.fingerprint(picked, epoch);
    if (epoch !== this.epoch) fail();
    if (replacement) { const old = this.entry(replacement); if (old.source.fingerprint !== fingerprint || old.source.size_bytes !== String(picked.size)) fail('必须重新选择与原会话完整校验相同的原件。'); old.path = picked.path; old.source.has_source = true; return { ...old.source }; }
    if (this.entries.size >= 4) fail('请先释放不再使用的本机记录，每次最多保留4份普通附件。');
    const handle = uuid(this.files.key());
    const source: NativeUploadSource = { handle, name: picked.name, media_type: picked.media, size_bytes: String(picked.size), fingerprint, upload_id: '', create_key: '', create_body: '', state: 'selected', has_source: true, prepared_at_ms: 0 };
    this.entries.set(handle, { source, path: picked.path }); return { ...source };
  }
  async prepare(handle: string, target: Pick<Schemas['UploadCreate'], 'folder_id' | 'name' | 'file_id' | 'base_version_id'>) {
    const e = this.entry(handle); if (e.source.upload_id || e.source.create_body || !target.name.trim()) fail();
    e.source.create_body = JSON.stringify({ ...target, size_bytes: e.source.size_bytes, media_type: e.source.media_type, source_fingerprint: e.source.fingerprint });
    e.source.name = target.name; e.source.create_key = this.files.key(); e.source.prepared_at_ms = Date.now(); e.source.state = 'prepared'; return { ...e.source };
  }
  async bind(handle: string, upload: Schemas['Upload']) {
    const e = this.entry(handle); if (upload.source_fingerprint !== e.source.fingerprint || upload.size_bytes !== e.source.size_bytes || Number(upload.size_bytes) > MINI_UPLOAD_BYTES || e.source.upload_id && e.source.upload_id !== upload.id) fail();
    if (e.source.create_body) { const b = JSON.parse(e.source.create_body) as Schemas['UploadCreate']; if (['folder_id', 'name', 'media_type', 'file_id', 'base_version_id'].some(k => ((b as unknown as Record<string, unknown>)[k] ?? null) !== ((upload as unknown as Record<string, unknown>)[k] ?? null))) fail(); }
    e.upload = upload; e.source.upload_id = upload.id; return { ...e.source };
  }
  async begin(handle: string) { const e = this.entry(handle); this.pause(); e.run = this.files.key(); e.source.state = 'checking'; return e.run; }
  async touch(handle: string, run: string) { if (this.entry(handle).run !== run) fail(); }
  async verifySource(handle: string) { const e = this.entry(handle); if (await this.fingerprint({ path: e.path, size: Number(e.source.size_bytes), name: e.source.name, media: e.source.media_type }, this.epoch) !== e.source.fingerprint) fail(); }
  async transfer(handle: string, run: string, number: number, permit: Schemas['SignedParts']['items'][number]) {
    const e = this.entry(handle), epoch = this.epoch, upload = e.upload;
    if (!upload) throw new MobileError('MINI_UPLOAD_CHANGED', '请先关联准确上传会话。');
    if (e.run !== run || !upload || permit.part_number !== number || permit.method !== 'PUT' || (!Number.isFinite(Date.parse(permit.expires_at)) || Date.parse(permit.expires_at) <= Date.now()) || !/^https:\/\//.test(permit.url)) fail();
    await this.verifySource(handle); const raw = new Uint8Array(await this.files.read(e.path));
    let part: Uint8Array | undefined;
    try {
      if (epoch !== this.epoch || e.run !== run || raw.length !== Number(e.source.size_bytes)) fail();
      const start = (number - 1) * upload.part_bytes; part = raw.slice(start, Math.min(raw.length, start + upload.part_bytes)); if (!part.length) fail();
      const headers: Record<string, string> = {}; for (const h of permit.headers ?? []) { if (/^(authorization|cookie|proxy-authorization)$/i.test(h.name)) fail(); headers[h.name] = h.value; }
      const request = this.files.put(permit.url, headers, part.buffer as ArrayBuffer); this.abort = request.abort;
      const result = await request.promise; if (epoch !== this.epoch || e.run !== run || result.status < 200 || result.status >= 300 || !result.etag) fail();
      return { part_number: number, size_bytes: String(part.length), etag: result.etag };
    } finally { raw.fill(0); part?.fill(0); this.abort = undefined; }
  }
  async finish(handle: string, run: string, state: string) { const e = this.entries.get(handle); if (e?.run === run) { e.run = undefined; e.source.state = state; } }
  pause() { this.abort?.(); this.abort = undefined; for (const e of this.entries.values()) e.run = undefined; }
  async forget(handle: string) { const e = this.entry(handle); if (e.run) fail('请先暂停传输。'); this.entries.delete(handle); }
}
