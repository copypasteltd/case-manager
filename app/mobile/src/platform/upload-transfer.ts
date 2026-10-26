import { MobileError, Scope, type MobileClient, type Schemas } from './session';

export type Upload = Schemas['Upload'];
export type NativeUploadSource = {
  handle: string; name: string; media_type: string; size_bytes: string; fingerprint: string;
  upload_id: string; create_key: string; create_body: string; state: string; has_source: boolean;
  prepared_at_ms: number;
};
export type NativeUploadEvent = { handle: string; run: string; state: string; sent_bytes: string; confirmed_bytes: string; size_bytes: string };
export interface NativeUploadPort {
  scope(owner: string): Promise<void>;
  list(): Promise<NativeUploadSource[]>;
  begin(handle: string): Promise<string>;
  bind(handle: string, upload: Upload): Promise<NativeUploadSource>;
  touch(handle: string, run: string, confirmed: string, transferred: string): Promise<void>;
  transfer(handle: string, run: string, number: number, permit: Schemas['SignedParts']['items'][number]): Promise<Schemas['AckParts']['parts'][number]>;
  verifySource(handle: string): Promise<void>;
  finish(handle: string, run: string, state: string): Promise<void>;
  pause(): void;
}
type Client = Pick<MobileClient, 'request' | 'operationKey' | 'epoch' | 'authenticated' | 'subscribe'>;
export type TransferState = Readonly<{
  handle: string | null; running: boolean; phase: string; upload: Upload | null;
  confirmed: number; transferred: number; currentPartBytes: number; size: number; error: unknown;
}>;
const MAX_BYTES = 120_000_000_000;
const UUID = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function invalid(message = '上传资料发生变化，请刷新队列并重新核对。'): never { throw new MobileError('UPLOAD_CHANGED', message); }
function layout(upload: Upload) {
  const size = Number(upload.size_bytes);
  if (!UUID.test(upload.id) || !Number.isSafeInteger(size) || size < 1 || size > MAX_BYTES || String(size) !== upload.size_bytes ||
      !Number.isSafeInteger(upload.version) || upload.version < 1 || !Number.isSafeInteger(upload.storage_epoch) || upload.storage_epoch < 1 ||
      !Number.isInteger(upload.part_bytes) || upload.part_bytes < 5 * 1024 * 1024 || upload.part_bytes > 512 * 1024 * 1024 ||
      !Number.isInteger(upload.part_count) || upload.part_count !== Math.ceil(size / upload.part_bytes) || upload.part_count > 10_000 || !Array.isArray(upload.confirmed_parts)) invalid();
  return size;
}
export function uploadedBytes(upload: Upload, parts: readonly number[] = upload.confirmed_parts): number {
  const size = layout(upload), seen = new Set<number>(); let count = 0;
  for (const part of parts) {
    if (!Number.isInteger(part) || part < 1 || part > upload.part_count || seen.has(part)) invalid();
    seen.add(part); count += Math.min(upload.part_bytes, size - (part - 1) * upload.part_bytes);
  }
  return count;
}
function match(upload: Upload, initial: Upload, source: NativeUploadSource) {
  layout(upload); uploadedBytes(upload);
  for (const key of ['id', 'folder_id', 'file_id', 'base_version_id', 'name', 'media_type', 'size_bytes', 'source_fingerprint', 'part_bytes', 'part_count', 'storage_epoch'] as const) if (upload[key] !== initial[key]) invalid();
  if (upload.source_fingerprint !== source.fingerprint || upload.size_bytes !== source.size_bytes || upload.version < initial.version) invalid();
}

/** Native owns file I/O and service lifetime; existing MobileClient owns auth rotation and every API command. */
export class UploadTransferController {
  private listeners = new Set<() => void>();
  private active: { scope: Scope; epoch: number; handle: string; run: string; finishing: boolean; paused: boolean } | null = null;
  private value: TransferState = { handle: null, running: false, phase: 'idle', upload: null, confirmed: 0, transferred: 0, currentPartBytes: 0, size: 0, error: undefined };
  constructor(private client: Client, private native: NativeUploadPort, private beforeStart: () => Promise<void> = async () => {}) {
    client.subscribe(() => { if (!client.authenticated) { this.pause(); this.value = { ...this.value, handle: null, upload: null, size: 0, confirmed: 0, transferred: 0, currentPartBytes: 0, error: undefined }; this.notify(); } });
  }
  snapshot = () => this.value;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private notify() { for (const listener of this.listeners) listener(); }
  private update(value: Partial<TransferState>) { this.value = { ...this.value, ...value }; this.notify(); }
  event(value: NativeUploadEvent) {
    const active = this.active;
    if (!active || value.handle !== active.handle || value.run !== active.run || active.finishing) return;
    if (value.state !== 'transferring') { active.paused = true; active.scope.close(); this.update({ phase: 'paused', currentPartBytes: 0 }); return; }
    const sent = Number(value.sent_bytes);
    if (Number.isSafeInteger(sent) && sent >= 0 && sent <= this.value.size) this.update({ currentPartBytes: sent });
  }
  pause() { const active = this.active; if (active) { active.paused = true; active.scope.close(); this.native.pause(); this.update({ phase: 'paused', currentPartBytes: 0 }); } }
  async start(handle: string, owner: string): Promise<Upload> {
    if (this.active) throw new MobileError('UPLOAD_BUSY', '当前已有一个文件正在传输，请先暂停它。');
    if (!UUID.test(handle) || !UUID.test(owner) || !this.client.authenticated) invalid();
    const active = { scope: new Scope(), epoch: this.client.epoch, handle, run: '', finishing: false, paused: false };
    this.active = active; this.update({ handle, running: true, phase: 'checking', upload: null, confirmed: 0, transferred: 0, currentPartBytes: 0, error: undefined });
    let finalState = 'uncertain';
    const current = () => { if (active.scope.closed || active.epoch !== this.client.epoch || !this.client.authenticated) throw new MobileError('UPLOAD_PAUSED', '本机上传已停止，继续时会先核对云端已有分片。'); };
    const read = async <T,>(path: string) => { current(); const result = await this.client.request<T>(path, { scope: active.scope }); current(); return result; };
    try {
      const me = await read<Schemas['Me']>('/me'); if (me.user.id !== owner) invalid();
      await this.native.scope(owner); current();
      const source = (await this.native.list()).find(value => value.handle === handle); current();
      if (!source || !source.has_source || !/^android-saf-v1:[a-f0-9]{64}$/.test(source.fingerprint)) invalid('请重新选择原文件并确认持久读取授权。');
      this.update({ size: Number(source.size_bytes) }); await this.beforeStart(); current();
      active.run = await this.native.begin(handle); current();
      await this.native.touch(handle, active.run, '0', '0'); current();
      let upload: Upload;
      if (source.upload_id) upload = await read<Upload>('/uploads/' + source.upload_id);
      else {
        if (!source.create_body || source.create_key.length < 16) invalid('请先核对目标目录并保存上传准备。');
        if (!Number.isSafeInteger(source.prepared_at_ms) || source.prepared_at_ms <= 0 || Date.now() - source.prepared_at_ms > 48 * 60 * 60_000 || source.prepared_at_ms > Date.now() + 300_000) invalid('原创建准备已超过有效时间，请先查服务端本人队列。需要新上传时，再明确释放本机记录并重新准备。');
        const body = JSON.parse(source.create_body) as Schemas['UploadCreate'];
        if (body.size_bytes !== source.size_bytes || body.source_fingerprint !== source.fingerprint) invalid();
        current(); this.update({ phase: 'creating' });
        upload = await this.client.request<Upload>('/uploads', { method: 'POST', key: source.create_key, body, scope: active.scope }); current();
      }
      layout(upload); if (upload.size_bytes !== source.size_bytes || upload.source_fingerprint !== source.fingerprint) invalid();
      if (source.create_body) {
        const original = JSON.parse(source.create_body) as Schemas['UploadCreate'];
        for (const key of ['folder_id', 'name', 'media_type'] as const) if (original[key] !== upload[key]) invalid();
        if ((original.file_id ?? null) !== upload.file_id || (original.base_version_id ?? null) !== upload.base_version_id) invalid();
      }
      const initial = upload;
      await this.native.bind(handle, upload); current();
      const snapshot = (value: Upload) => {
        match(value, initial, source); upload = value;
        this.update({ upload: value, confirmed: uploadedBytes(value), currentPartBytes: 0 });
      };
      snapshot(upload);
      if (upload.state !== 'uploading') { finalState = 'awaiting_verification'; this.update({ phase: 'server_processing' }); return upload; }
      const command = async <T,>(action: 'resume' | 'sign-parts' | 'ack-parts' | 'finalize', body: unknown) => {
        current(); await this.native.touch(handle, active.run, String(this.value.confirmed), String(this.value.transferred)); current();
        // MobileClient refreshes memory-only credentials before signing each part.
        const result = await this.client.request<T>('/uploads/' + upload.id + '/' + action, { method: 'POST', key: this.client.operationKey(), body, scope: active.scope }); current(); return result;
      };
      this.update({ phase: 'reconciling' });
      snapshot(await command<Upload>('resume', { expected_version: upload.version, source_fingerprint: source.fingerprint }));
      if (upload.state !== 'uploading') invalid();
      const transferred = new Set(upload.confirmed_parts); this.update({ transferred: uploadedBytes(upload) });
      for (let number = 1; number <= upload.part_count; number++) {
        if (transferred.has(number)) continue;
        this.update({ phase: 'signing' });
        const permits = await command<Schemas['SignedParts']>('sign-parts', { expected_version: upload.version, part_numbers: [number] });
        if (!permits || !Array.isArray(permits.items) || permits.items.length !== 1 || permits.items[0].part_number !== number) invalid('分片许可与本次请求不一致，请核对上传状态。');
        this.update({ phase: 'transferring', currentPartBytes: 0 });
        const receipt = await this.native.transfer(handle, active.run, number, permits.items[0]); current();
        const expected = Math.min(upload.part_bytes, Number(upload.size_bytes) - (number - 1) * upload.part_bytes);
        if (receipt.part_number !== number || receipt.size_bytes !== String(expected) || typeof receipt.etag !== 'string' || !receipt.etag || receipt.etag.length > 200 || /[\u0000-\u001f\u007f]/.test(receipt.etag)) invalid('分片回执异常，请先对账云端状态。');
        this.update({ phase: 'acknowledging' });
        snapshot(await command<Upload>('ack-parts', { expected_version: upload.version, parts: [receipt] }));
        transferred.add(number); this.update({ transferred: uploadedBytes(upload, [...transferred]), currentPartBytes: 0 });
      }
      this.update({ phase: 'reconciling' });
      await this.native.verifySource(handle); current();
      snapshot(await command<Upload>('resume', { expected_version: upload.version, source_fingerprint: source.fingerprint }));
      if (upload.confirmed_parts.length !== upload.part_count || uploadedBytes(upload) !== Number(source.size_bytes)) invalid('云端尚未确认全部分片，请刷新后继续核对。');
      this.update({ phase: 'finalizing', transferred: Number(source.size_bytes) });
      snapshot(await command<Upload>('finalize', { expected_version: upload.version }));
      if (!['finalizing', 'verifying', 'ready'].includes(upload.state)) invalid();
      finalState = 'awaiting_verification'; this.update({ phase: 'server_processing' }); return upload;
    } catch (cause) {
      finalState = active.paused || active.scope.closed ? 'paused' : this.client.authenticated ? 'uncertain' : 'waiting_session';
      const error = cause instanceof MobileError ? cause : new MobileError('NATIVE_UPLOAD_UNCONFIRMED', '原生文件操作结果未确认。请刷新队列；权限变化时重新选择原文件，继续时先核对云端分片。');
      if (active.epoch === this.client.epoch) this.update({ error, phase: finalState });
      throw error;
    } finally {
      active.finishing = true; active.scope.close();
      if (active.run) await this.native.finish(handle, active.run, finalState).catch(() => {});
      if (this.active === active) this.active = null;
      this.update({ running: false, currentPartBytes: 0 });
    }
  }
}
