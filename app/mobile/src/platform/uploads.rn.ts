import { NativeEventEmitter, NativeModules, PermissionsAndroid, Platform } from 'react-native';
import { client } from './taro';
import { MobileError, type Schemas } from './session';
import { UploadTransferController, type NativeUploadSource, type NativeUploadEvent, type NativeUploadPort, type Upload } from './upload-transfer';

interface Bridge {
  scope(owner: string): Promise<void>; clearScope(): void; list(): Promise<string>;
  select(replacement: string | null): Promise<string>; prepare(handle: string, input: string): Promise<string>;
  generated(name: string, media: string, content: string, sha: string): Promise<string>;
  bind(handle: string, input: string): Promise<string>; begin(handle: string): Promise<string>;
  touch(handle: string, run: string, confirmed: string, transferred: string): Promise<void>;
  transfer(handle: string, run: string, number: number, input: string): Promise<string>;
  verifySource(handle: string): Promise<void>; finish(handle: string, run: string, state: string): Promise<void>;
  pause(): void; forget(handle: string): Promise<void>; addListener(name: string): void; removeListeners(count: number): void;
}
const bridge = NativeModules.CPUpload as Bridge;
const UUID = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function source(input: unknown): NativeUploadSource {
  if (!input || typeof input !== 'object') throw new MobileError('INVALID_RESPONSE', '本机上传记录不完整。');
  const value = input as NativeUploadSource, size = Number(value.size_bytes);
  if (!UUID.test(value.handle) || typeof value.name !== 'string' || value.name.length > 255 || typeof value.media_type !== 'string' ||
      !Number.isSafeInteger(size) || size < 1 || size > 120_000_000_000 || String(size) !== value.size_bytes ||
      !/^(android-saf-v1|office-artifact-v1):[a-f0-9]{64}$/.test(value.fingerprint) || value.fingerprint.startsWith('office-artifact-v1:') && size > 12 * 1024 * 1024 || typeof value.has_source !== 'boolean' ||
      typeof value.state !== 'string' || typeof value.upload_id !== 'string' || value.upload_id !== '' && !UUID.test(value.upload_id) ||
      !Number.isSafeInteger(value.prepared_at_ms) || value.prepared_at_ms < 0 || typeof value.create_key !== 'string' || typeof value.create_body !== 'string' || value.create_body.length > 4096) throw new MobileError('INVALID_RESPONSE', '本机上传记录无法核对。');
  return value;
}
async function safe<T>(action: () => Promise<T>): Promise<T> {
  try { return await action(); }
  catch (error) {
    if (error instanceof MobileError) throw error;
    const code = error && typeof error === 'object' && 'code' in error ? String(error.code) : '';
    if (code === 'UPLOAD_CANCELLED') throw new MobileError(code, '已取消文件选择。');
    throw new MobileError('NATIVE_UPLOAD_UNCONFIRMED', '本机文件操作结果未确认。请刷新队列；文件权限变化时重新选择原文件。');
  }
}
export const nativeUploads: NativeUploadPort & {
  select(replacement?: string): Promise<NativeUploadSource>;
  generated(artifact: Schemas['OfficeArtifact']): Promise<NativeUploadSource>;
  prepare(handle: string, target: Pick<Schemas['UploadCreate'], 'folder_id' | 'name' | 'file_id' | 'base_version_id'>): Promise<NativeUploadSource>;
  forget(handle: string): Promise<void>;
} = {
  scope: owner => safe(() => bridge.scope(owner)),
  list: () => safe(async () => { const value = JSON.parse(await bridge.list()) as unknown; if (!Array.isArray(value) || value.length > 16) throw new Error(); return value.map(source); }),
  select: replacement => safe(async () => source(JSON.parse(await bridge.select(replacement ?? null)))),
  generated: artifact => safe(async () => source(JSON.parse(await bridge.generated(artifact.name, artifact.media_type, artifact.content_base64, artifact.sha256)))),
  prepare: (handle, target) => safe(async () => source(JSON.parse(await bridge.prepare(handle, JSON.stringify(target))))),
  bind: (handle, upload: Upload) => safe(async () => source(JSON.parse(await bridge.bind(handle, JSON.stringify(upload))))),
  begin: handle => safe(() => bridge.begin(handle)),
  touch: (handle, run, confirmed, transferred) => safe(() => bridge.touch(handle, run, confirmed, transferred)),
  transfer: (handle, run, number, permit) => safe(async () => {
    const expires = Date.parse(permit.expires_at); if (!Number.isFinite(expires)) throw new Error();
    return JSON.parse(await bridge.transfer(handle, run, number, JSON.stringify({ ...permit, expires_ms: expires }))) as Schemas['AckParts']['parts'][number];
  }),
  verifySource: handle => safe(() => bridge.verifySource(handle)),
  finish: (handle, run, state) => safe(() => bridge.finish(handle, run, state)),
  pause: () => bridge.pause(),
  forget: handle => safe(() => bridge.forget(handle)),
};
async function notifications() {
  if (Number(Platform.Version) < 33) return;
  const permission = PermissionsAndroid.PERMISSIONS.POST_NOTIFICATIONS;
  const allowed = await PermissionsAndroid.check(permission) || await PermissionsAndroid.request(permission) === PermissionsAndroid.RESULTS.GRANTED;
  if (!allowed) throw new MobileError('UPLOAD_NOTIFICATIONS_REQUIRED', '请在系统设置中允许上传通知，再回到应用前台继续。通知中可查看进度和暂停传输。');
}
export const uploadTransfer = new UploadTransferController(client, nativeUploads, notifications);
export function observeNativeUploads() {
  const subscription = new NativeEventEmitter(bridge).addListener('CPUploadStatus', (event: NativeUploadEvent) => uploadTransfer.event(event));
  return () => { subscription.remove(); uploadTransfer.pause(); };
}
