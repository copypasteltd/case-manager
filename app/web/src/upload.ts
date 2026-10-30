import type { Schemas } from './api';

export type UploadSession = Schemas['Upload'];
type PartPermit = Schemas['SignedParts']['items'][number];
export type UploadSource = Pick<File, 'name' | 'size' | 'lastModified' | 'slice'>;
const MAX_BYTES = 120_000_000_000;
const MIN_PART = 5 * 1024 * 1024;
const MAX_PART = 512 * 1024 * 1024;
const SAMPLE = 64 * 1024;

function invalid(message: string): never { throw new Error(message); }
export function partBounds(upload: UploadSession, number: number): { start: number; end: number } {
  const size = Number(upload.size_bytes);
  if (!Number.isSafeInteger(size) || size < 1 || size > MAX_BYTES || String(size) !== upload.size_bytes ||
      !Number.isSafeInteger(upload.part_bytes) || upload.part_bytes < MIN_PART || upload.part_bytes > MAX_PART ||
      upload.part_count !== Math.ceil(size / upload.part_bytes) || upload.part_count > 10_000 ||
      !Number.isInteger(number) || number < 1 || number > upload.part_count) invalid('上传布局无效，请刷新状态。');
  const start = (number - 1) * upload.part_bytes;
  return { start, end: Math.min(start + upload.part_bytes, size) };
}
export function confirmedBytes(upload: UploadSession): number {
  const seen = new Set<number>();
  return upload.confirmed_parts.reduce((sum, number) => {
    if (seen.has(number)) invalid('服务端分片进度重复，请刷新状态。');
    seen.add(number);
    const { start, end } = partBounds(upload, number); return sum + end - start;
  }, 0);
}

// A bounded re-selection aid, not a full-file checksum or authenticity proof.
// Only the server's frozen-source SHA-256 is authoritative.
export async function sourceFingerprint(source: UploadSource, signal?: AbortSignal, expected?: string): Promise<string> {
  signal?.throwIfAborted();
  if (!Number.isSafeInteger(source.size) || source.size < 1 || source.size > MAX_BYTES ||
      !Number.isSafeInteger(source.lastModified) || source.lastModified < 0) invalid('文件需为 1 字节至 120 GB，请重新选择。');
  if (expected?.startsWith('office-artifact-v1:')) {
    if (source.size > 12 * 1024 * 1024 || !/^office-artifact-v1:[a-f0-9]{64}$/.test(expected)) invalid('Office生成文件超过大小限制。');
    const hash = await crypto.subtle.digest('SHA-256', await source.slice(0, source.size).arrayBuffer()); signal?.throwIfAborted();
    return 'office-artifact-v1:' + [...new Uint8Array(hash)].map(value => value.toString(16).padStart(2, '0')).join('');
  }
  const head = new Uint8Array(await source.slice(0, Math.min(SAMPLE, source.size)).arrayBuffer());
  signal?.throwIfAborted();
  const tail = new Uint8Array(await source.slice(Math.max(head.byteLength, source.size - SAMPLE), source.size).arrayBuffer());
  const metadata = new TextEncoder().encode(JSON.stringify([source.name, source.size, source.lastModified]));
  const bytes = new Uint8Array(metadata.byteLength + head.byteLength + tail.byteLength);
  bytes.set(metadata); bytes.set(head, metadata.byteLength); bytes.set(tail, metadata.byteLength + head.byteLength);
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  signal?.throwIfAborted();
  return 'web-sample-v1:' + [...new Uint8Array(digest)].map(value => value.toString(16).padStart(2, '0')).join('');
}

function validatePermit(permit: PartPermit, number: number, origin: string): Record<string, string> {
  let target: URL;
  try { target = new URL(permit.url); } catch { return invalid('分片上传许可无效。'); }
  const official = target.hostname.endsWith('.aliyuncs.com') || target.hostname.endsWith('.r2.cloudflarestorage.com');
  const expiry = Date.parse(permit.expires_at);
  if (!official || target.protocol !== 'https:' || target.origin === origin || target.username || target.password || target.hash ||
      (target.port && target.port !== '443') || permit.method !== 'PUT' || permit.part_number !== number ||
      !Number.isFinite(expiry) || expiry < Date.now() + 10_000 || expiry > Date.now() + 330_000) invalid('分片上传许可已失效，请重新核对上传状态。');
  const headers: Record<string, string> = {};
  for (const { name, value } of permit.headers ?? []) {
    const normalized = name.toLowerCase();
    if (!/^[a-z0-9-]+$/.test(normalized) || /[\r\n]/.test(value) ||
        !(['content-type', 'content-md5'].includes(normalized) || normalized.startsWith('x-amz-') || normalized.startsWith('x-oss-')) ||
        normalized in headers) invalid('分片上传所需请求头无效。');
    headers[normalized] = value;
  }
  return headers;
}

async function putPart(source: UploadSource, upload: UploadSession, permit: PartPermit, number: number, signal: AbortSignal, origin: string): Promise<Schemas['AckParts']['parts'][number]> {
  const { start, end } = partBounds(upload, number);
  const headers = validatePermit(permit, number, origin);
  const body = source.slice(start, end);
  if (body.size !== end - start) invalid('本机文件发生变化，请重新选择原文件。');
  const timeout = AbortSignal.timeout(10 * 60_000);
  const bounded = AbortSignal.any([signal, timeout]);
  let response: Response;
  try {
    response = await fetch(permit.url, { method: 'PUT', body, headers, credentials: 'omit', cache: 'no-store',
      redirect: 'error', referrerPolicy: 'no-referrer', signal: bounded });
  } catch {
    signal.throwIfAborted();
    invalid('分片传输中断，结果尚未确认。继续时将先核对云端已有分片。');
  }
  // Provider error bodies and signed addresses must never enter UI or logs.
  if (!response!.ok || response!.status !== 200) {
    await response!.body?.cancel();
    invalid('云端未确认此分片，请核对连接和上传状态后继续。');
  }
  const etag = response!.headers.get('ETag');
  await response!.body?.cancel();
  if (!etag || etag.length > 200 || /[\u0000-\u001f\u007f]/.test(etag)) invalid('无法读取云端分片回执，请管理员检查上传跨域配置后继续。');
  return { part_number: number, etag, size_bytes: String(end - start) };
}

export type UploadTransferOptions = {
  upload: UploadSession;
  source: UploadSource;
  signal: AbortSignal;
  origin: string;
  ensureSession: () => Promise<void>;
  command: <T>(action: 'resume' | 'sign-parts' | 'ack-parts' | 'finalize', body: unknown) => Promise<T>;
  snapshot: (upload: UploadSession) => void;
  progress: (completedBytes: number, total: number) => void;
};

// At most two bounded Blob slices are in flight. No whole-file read, storage
// credentials, persistent URLs or automatic retry of an uncertain mutation.
export async function transferUpload(options: UploadTransferOptions): Promise<UploadSession> {
  const { source, signal, command, snapshot, progress, ensureSession } = options;
  let upload = options.upload;
  signal.throwIfAborted();
  if (upload.state !== 'uploading' || String(source.size) !== upload.size_bytes ||
      await sourceFingerprint(source, signal, upload.source_fingerprint) !== upload.source_fingerprint) invalid('所选文件与原上传源不一致，请重新选择原文件。');
  partBounds(upload, 1);
  await ensureSession();
  upload = await command<UploadSession>('resume', { expected_version: upload.version, source_fingerprint: upload.source_fingerprint });
  if (upload.id !== options.upload.id || upload.state !== 'uploading' || upload.source_fingerprint !== options.upload.source_fingerprint ||
      upload.size_bytes !== options.upload.size_bytes || upload.part_bytes !== options.upload.part_bytes || upload.part_count !== options.upload.part_count) invalid('上传会话发生变化，请刷新后核对。');
  snapshot(upload);
  let completed = confirmedBytes(upload);
  progress(completed, source.size);
  const confirmed = new Set(upload.confirmed_parts);
  const missing = Array.from({ length: upload.part_count }, (_, i) => i + 1).filter(number => !confirmed.has(number));
  for (let offset = 0; offset < missing.length; offset += 2) {
    signal.throwIfAborted(); await ensureSession();
    const numbers = missing.slice(offset, offset + 2);
    const permits = await command<Schemas['SignedParts']>('sign-parts', { expected_version: upload.version, part_numbers: numbers });
    if (permits.items.length !== numbers.length || new Set(permits.items.map(item => item.part_number)).size !== numbers.length) invalid('云端分片许可不完整，请刷新状态。');
    const batch = new AbortController(), combined = AbortSignal.any([signal, batch.signal]);
    const results = await Promise.allSettled(numbers.map(async number => {
      const permit = permits.items.find(item => item.part_number === number);
      if (!permit) invalid('云端分片许可与请求不一致。');
      try {
        const ack = await putPart(source, upload, permit!, number, combined, options.origin);
        completed += Number(ack.size_bytes); progress(completed, source.size); return ack;
      } catch (error) { batch.abort(); throw error; }
    }));
    signal.throwIfAborted();
    const failure = results.find(result => result.status === 'rejected');
    if (failure?.status === 'rejected') throw failure.reason;
    const parts = results.map(result => (result as PromiseFulfilledResult<Schemas['AckParts']['parts'][number]>).value);
    upload = await command<UploadSession>('ack-parts', { expected_version: upload.version, parts });
    snapshot(upload);
  }
  signal.throwIfAborted(); await ensureSession();
  // The client receipt is only a hint. ListParts is authoritative and must
  // reconcile the complete layout before requesting source freezing.
  upload = await command<UploadSession>('resume', { expected_version: upload.version, source_fingerprint: upload.source_fingerprint });
  snapshot(upload);
  if (upload.confirmed_parts.length !== upload.part_count || confirmedBytes(upload) !== source.size) invalid('云端尚未确认全部分片，请核对状态后继续。');
  signal.throwIfAborted();
  const frozen = await command<UploadSession>('finalize', { expected_version: upload.version });
  snapshot(frozen);
  return frozen;
}
