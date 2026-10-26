import Taro from '@tarojs/taro';
import { decodeContentText, type ContentFormat, type ContentSink } from './content';
import { MobileError } from './session';

export type ContentArtifact = { kind: 'text'; text: string; dispose(): Promise<void> } | { kind: 'image' | 'document'; url: string; extension: string; hold?(): () => void; dispose(): Promise<void> };
const native = process.env.TARO_ENV !== 'h5';
const ownedName = /^cp-read-[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}-[a-z0-9]+-[0-9]+\.(png|jpg|gif|webp|pdf|docx?|xlsx?|pptx?)$/;
let serial = 0;
let staleCleanup: Promise<void> | undefined;
function nativeDirectory() { const value = Taro.env.USER_DATA_PATH; if (!value) throw new Error('Native file storage is unavailable'); return value; }
async function cleanStaleFiles() {
  if (!native) return;
  const fs = Taro.getFileSystemManager(), directory = nativeDirectory();
  const listing = await new Promise<string[]>((resolve, reject) => fs.readdir({ dirPath: directory, success: value => resolve(value.files), fail: reject }));
  for (const name of listing.filter(name => ownedName.test(name))) await new Promise<void>((resolve, reject) => fs.unlink({ filePath: directory + '/' + name, success: () => resolve(), fail: reject }));
}
export function prepareContentStorage() { return staleCleanup ??= cleanStaleFiles(); }
export async function createContentSink(format: ContentFormat, permitlessID: string): Promise<ContentSink<ContentArtifact>> {
  // Names are derived only from a fixed-version UUID, never a user filename.
  if (!/^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/.test(permitlessID)) throw new MobileError('INVALID_RESOURCE', '文件版本无效。');
  let disposed = false, length = 0;
  const chunks: Uint8Array[] = [];
  let objectURL: string | undefined, path: string | undefined;
  let pending: Promise<void> = Promise.resolve();
  let abortFlight: Promise<void> | undefined;
  let leases = 0;
  const waiters: (() => void)[] = [];
  const hold = () => {
    if (disposed) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束。');
    leases++; let released = false;
    return () => { if (released) return; released = true; if (--leases === 0) for (const resolve of waiters.splice(0)) resolve(); };
  };
  const fs = native && format.kind !== 'text' ? Taro.getFileSystemManager() : undefined;
  const remove = async () => {
    if (objectURL) { URL.revokeObjectURL(objectURL); objectURL = undefined; }
    chunks.length = 0;
    if (path && fs) { const own = path; await new Promise<void>((resolve, reject) => fs.unlink({ filePath: own, success: () => resolve(), fail: () => reject(new MobileError('CONTENT_CLEANUP_FAILED', '临时文件清理失败，请重新启动客户端后重试。')) })); path = undefined; }
  };
  const abort = () => { disposed = true; return abortFlight ??= (async () => { await pending.catch(() => {}); if (leases) await new Promise<void>(resolve => waiters.push(resolve)); await remove(); })(); };
  if (fs) {
    try {
      // One startup sweep is completed before any new owned content exists.
      await prepareContentStorage();
      if (typeof fs.appendFile !== 'function') throw new Error('missing appendFile');
      path = nativeDirectory() + '/cp-read-' + permitlessID + '-' + Date.now().toString(36) + '-' + (++serial) + '.' + format.extension;
      await new Promise<void>((resolve, reject) => fs.writeFile({ filePath: path!, data: new ArrayBuffer(0), success: () => resolve(), fail: reject }));
    } catch { await abort(); throw new MobileError('CONTENT_STORAGE_UNAVAILABLE', '无法建立本端临时文件，请检查可用空间或使用 Web 下载原件。'); }
  }
  return {
    async write(bytes) {
      if (disposed || length + bytes.length > format.limit) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束或超出阅读范围。');
      length += bytes.length;
      if (fs && path) {
        const filePath = path;
        pending = new Promise<void>((resolve, reject) => fs.appendFile({ filePath, data: bytes.slice().buffer as ArrayBuffer, success: () => resolve(), fail: reject }));
        try { await pending; } catch { throw new MobileError('CONTENT_STORAGE_UNAVAILABLE', '临时文件写入失败，请检查可用空间后重试。'); }
        if (disposed) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束。');
      } else chunks.push(bytes);
    },
    async finish() {
      if (disposed) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束。');
      if (format.kind === 'text') {
        const result = new Uint8Array(length); let at = 0; for (const bytes of chunks) { result.set(bytes, at); at += bytes.length; }
        const text = decodeContentText(result); result.fill(0); chunks.length = 0;
        return { kind: 'text', text, dispose: abort };
      }
      if (path) return { kind: format.kind, url: path, extension: format.extension, hold, dispose: abort };
      objectURL = URL.createObjectURL(new Blob(chunks.map(bytes => bytes.slice().buffer as ArrayBuffer), { type: format.mime })); chunks.length = 0;
      return { kind: format.kind, url: objectURL, extension: format.extension, dispose: abort };
    }, abort,
  };
}
