import { NativeModules } from 'react-native';
import { decodeContentText, type ContentFormat, type ContentSink } from './content';
import { MobileError } from './session';

export type ContentArtifact = { kind: 'text'; text: string; dispose(): Promise<void> } | { kind: 'image' | 'document'; url: string; extension: string; hold?(): () => void; dispose(): Promise<void> };
interface NativeCache {
  clear(): Promise<void>;
  create(version: string, extension: string): Promise<string>;
  append(handle: string, base64: string): Promise<void>;
  finish(handle: string, bytes: number): Promise<string>;
  remove(handle: string): Promise<void>;
}
const cache = () => NativeModules.CPReadCache as NativeCache;
let cleanup: Promise<void> | undefined;
export function prepareContentStorage() { return cleanup ??= cache().clear(); }
const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
function encode(bytes: Uint8Array) {
  let out = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i], b = bytes[i + 1], c = bytes[i + 2];
    out += alphabet[a >>> 2] + alphabet[((a & 3) << 4) | ((b ?? 0) >>> 4)] + (b === undefined ? '=' : alphabet[((b & 15) << 2) | ((c ?? 0) >>> 6)]) + (c === undefined ? '=' : alphabet[c & 63]);
  }
  return out;
}
export async function createContentSink(format: ContentFormat, version: string): Promise<ContentSink<ContentArtifact>> {
  if (!/^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/.test(version)) throw new MobileError('INVALID_RESOURCE', '文件版本无效。');
  await prepareContentStorage();
  let disposed = false, length = 0, leases = 0;
  let pending = Promise.resolve(), abortFlight: Promise<void> | undefined;
  const chunks: Uint8Array[] = [], waiters: (() => void)[] = [];
  const handle = format.kind === 'text' ? undefined : await cache().create(version, format.extension);
  const abort = () => {
    disposed = true;
    return abortFlight ??= (async () => {
      await pending.catch(() => {});
      if (leases) await new Promise<void>(resolve => waiters.push(resolve));
      for (const chunk of chunks) chunk.fill(0);
      chunks.length = 0;
      if (handle) await cache().remove(handle);
    })();
  };
  const hold = () => {
    if (disposed) throw new MobileError('REQUEST_CANCELLED', '本次读取已结束。');
    leases++; let released = false;
    return () => { if (released) return; released = true; if (--leases === 0) for (const release of waiters.splice(0)) release(); };
  };
  return {
    async write(bytes) {
      if (disposed || length + bytes.length > format.limit) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束或超出阅读范围。');
      length += bytes.length;
      if (!handle) { chunks.push(bytes.slice()); return; }
      pending = pending.then(async () => {
        for (let offset = 0; offset < bytes.length; offset += 65536) {
          if (disposed) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束。');
          await cache().append(handle, encode(bytes.subarray(offset, offset + 65536)));
        }
      });
      try { await pending; } catch { throw new MobileError('CONTENT_STORAGE_UNAVAILABLE', '临时文件写入失败，请检查可用空间后重试。'); }
    },
    async finish() {
      if (disposed) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束。');
      await pending;
      if (handle) return { kind: format.kind as 'image' | 'document', url: await cache().finish(handle, length), extension: format.extension, hold, dispose: abort };
      const bytes = new Uint8Array(length); let offset = 0;
      for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; chunk.fill(0); }
      chunks.length = 0;
      try { return { kind: 'text', text: decodeContentText(bytes), dispose: abort }; } finally { bytes.fill(0); }
    }, abort,
  };
}
