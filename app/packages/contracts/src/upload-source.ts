import { sha256 } from '@noble/hashes/sha2.js';

export const SOURCE_SHA256_PREFIX = 'source-sha256-v2:';
export const SOURCE_READ_BYTES = 1 << 20;
export const MAX_SOURCE_BYTES = 120_000_000_000;
export const isFullSourceFingerprint = (value: string): boolean => /^source-sha256-v2:[a-f0-9]{64}$/.test(value);

type Source = { size: number; slice(start: number, end: number): { arrayBuffer(): Promise<ArrayBuffer> } };

// Incremental hashing visits every byte before any resumed part is retained.
// No whole-file buffer or persisted file data; the server independently checks
// the final frozen object against this expectation before scan/promotion.
export async function fullSourceFingerprint(source: Source, signal?: AbortSignal): Promise<string> {
  if (!Number.isSafeInteger(source.size) || source.size < 1 || source.size > MAX_SOURCE_BYTES) throw new Error('文件需为1字节至120 GB。');
  const hash = sha256.create();
  try {
    for (let start = 0; start < source.size; start += SOURCE_READ_BYTES) {
      signal?.throwIfAborted();
      const end = Math.min(start + SOURCE_READ_BYTES, source.size);
      const bytes = new Uint8Array(await source.slice(start, end).arrayBuffer());
      try {
        signal?.throwIfAborted();
        if (bytes.byteLength !== end - start) throw new Error('无法完整读取原文件，请重新选择。');
        hash.update(bytes);
      } finally { bytes.fill(0); }
    }
    signal?.throwIfAborted();
    return SOURCE_SHA256_PREFIX + Array.from(hash.digest(), b => b.toString(16).padStart(2, '0')).join('');
  } finally { hash.destroy(); }
}
