import { sha256 } from '@noble/hashes/sha2.js';
import { MobileError, type Schemas, type Scope } from './session';

export const CONTENT_CHUNK_BYTES = 1 << 20;
export const CONTENT_PREVIEW_BYTES = 32 << 20;
export const CONTENT_TEXT_BYTES = 1 << 20;
export type ContentKind = 'text' | 'image' | 'document';
export type ContentFormat = { kind: ContentKind; extension: string; mime: string; limit: number };
export interface ContentSink<T> { write(bytes: Uint8Array): Promise<void>; finish(): Promise<T>; abort(): Promise<void> }
export interface ContentTransport {
  grant(versionID: string, scope: Scope): Promise<Schemas['DownloadResult']>;
  chunk(id: string, offset: number, length: number, sha: string, scope: Scope): Promise<Schemas['DownloadChunk']>;
}
const bad = (message = '文件内容与固定版本不一致，已停止展示。') => new MobileError('CONTENT_INTEGRITY_FAILED', message);
const isID = (v: string) => /^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/.test(v);
const hex = (bytes: Uint8Array) => Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
function size(value: string) {
  if (!/^(0|[1-9][0-9]*)$/.test(value)) throw bad();
  const result = Number(value); if (!Number.isSafeInteger(result)) throw bad(); return result;
}
export function contentFormat(version: Pick<Schemas['FileVersion'], 'original_name' | 'media_type'>): ContentFormat | null {
  const mime = version.media_type.toLowerCase().split(';')[0].trim(), name = version.original_name?.toLowerCase() ?? '';
  const images: Record<string, string> = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/gif': 'gif', 'image/webp': 'webp' };
  if (images[mime]) return { kind: 'image', extension: images[mime], mime, limit: CONTENT_PREVIEW_BYTES };
  if (mime === 'application/pdf') return { kind: 'document', extension: 'pdf', mime, limit: CONTENT_PREVIEW_BYTES };
  // Text is always rendered as literal text, including Markdown/JSON/CSV.
  if (['text/plain', 'text/markdown', 'text/csv', 'application/json'].includes(mime)) return { kind: 'text', extension: 'txt', mime: 'text/plain;charset=utf-8', limit: CONTENT_TEXT_BYTES };
  const office: Record<string, string> = { doc: 'application/msword', docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', xls: 'application/vnd.ms-excel', xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', ppt: 'application/vnd.ms-powerpoint', pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation' };
  const ext = name.split('.').at(-1) ?? '';
  if (office[ext] === mime) return { kind: 'document', extension: ext, mime, limit: CONTENT_PREVIEW_BYTES };
  return null;
}
export function contentEligibility(version: Pick<Schemas['FileVersion'], 'availability' | 'scan_state' | 'size_bytes'>, format: ContentFormat | null): string | null {
  if (version.availability !== 'available') return '此固定版本当前不可用，请刷新资料或联系管理员。';
  if (version.scan_state !== 'clean') return version.scan_state === 'waived' ? '此版本经扫描豁免放行，当前不提供预览。请通过 Web 核对风险并下载原件。' : '此版本尚未通过扫描，暂时不能读取内容。';
  if (!format) return '此格式暂不支持在本页阅读，请使用 Web 下载原件。';
  try { if (size(version.size_bytes) > format.limit) return '文件超出本端当前阅读范围，请使用 Web 下载原件。'; } catch { return '文件大小资料无效，请刷新后重试。'; }
  return null;
}
function assertSignature(bytes: Uint8Array, format: ContentFormat, total: number) {
  if (format.kind === 'text') return;
  const starts = (pattern: number[]) => pattern.every((b, i) => bytes[i] === b);
  const ascii = (from: number, value: string) => [...value].every((c, i) => bytes[from + i] === c.charCodeAt(0));
  const ext = format.extension;
  const valid = ext === 'png' ? starts([137, 80, 78, 71, 13, 10, 26, 10]) : ext === 'jpg' ? starts([255, 216, 255]) : ext === 'gif' ? ascii(0, 'GIF87a') || ascii(0, 'GIF89a') : ext === 'webp' ? ascii(0, 'RIFF') && ascii(8, 'WEBP') : ext === 'pdf' ? ascii(0, '%PDF-') : ext.endsWith('x') ? starts([80, 75, 3, 4]) : starts([208, 207, 17, 224, 161, 177, 26, 225]);
  if (!valid || total === 0) throw bad('文件内容与声明格式不符，已停止展示。');
}

// Portable, canonical base64 decoding; no Node Buffer/atob/native codec needed.
const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
export function decodeContentBase64(value: string, expected: number): Uint8Array {
  if (typeof value !== 'string' || value.length > 1398104 || value.length % 4 !== 0) throw bad();
  const padding = value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0;
  if (value.length / 4 * 3 - padding !== expected || expected > CONTENT_CHUNK_BYTES || expected < 0) throw bad();
  const result = new Uint8Array(expected);
  let out = 0;
  for (let i = 0; i < value.length; i += 4) {
    const last = i + 4 === value.length;
    const a = alphabet.indexOf(value[i]), b = alphabet.indexOf(value[i + 1]);
    const c = value[i + 2] === '=' && last && padding === 2 ? 0 : alphabet.indexOf(value[i + 2]);
    const d = value[i + 3] === '=' && last && padding > 0 ? 0 : alphabet.indexOf(value[i + 3]);
    if (Math.min(a, b, c, d) < 0 || (last && ((padding === 2 && (b & 15) !== 0) || (padding === 1 && (c & 3) !== 0)))) throw bad();
    result[out++] = (a << 2) | (b >> 4);
    if (out < expected) result[out++] = (b << 4) | (c >> 2);
    if (out < expected) result[out++] = (c << 6) | d;
  }
  return result;
}
export function decodeContentText(bytes: Uint8Array): string {
  if (bytes.length > CONTENT_TEXT_BYTES) throw bad();
  const parts: string[] = []; let part = '';
  for (let i = 0; i < bytes.length;) {
    const first = bytes[i++]; let code: number, count: number;
    if (first < 128) { code = first; count = 0; }
    else if (first >= 194 && first <= 223) { code = first & 31; count = 1; }
    else if (first >= 224 && first <= 239) { code = first & 15; count = 2; }
    else if (first >= 240 && first <= 244) { code = first & 7; count = 3; }
    else throw bad('文本不是有效的 UTF-8，请使用 Web 下载原件核对编码。');
    for (let n = 0; n < count; n++) { const next = bytes[i++]; if (next === undefined || (next & 192) !== 128) throw bad('文本编码不完整，已停止展示。'); code = (code << 6) | (next & 63); }
    if ((count === 1 && code < 128) || (count === 2 && code < 2048) || (count === 3 && code < 65536) || code > 0x10ffff || (code >= 0xd800 && code <= 0xdfff) || code === 0) throw bad('文本编码不适合直接阅读，请下载原件核对。');
    if (!(i === count + 1 && code === 0xfeff)) part += String.fromCodePoint(code);
    if (part.length > 4096) { parts.push(part); part = ''; }
  }
  parts.push(part); return parts.join('');
}

export async function readFixedContent<T>(input: { version: Schemas['FileVersion']; format: ContentFormat; transport: ContentTransport; scope: Scope; sink: ContentSink<T>; progress?(received: number, total: number): void; now?: () => number }): Promise<T> {
  const { version, format, scope, sink, transport } = input;
  const live = () => { if (scope.closed) throw new MobileError('REQUEST_CANCELLED', '本次文件读取已结束。'); };
  const release = scope.own(() => { void sink.abort().catch(() => {}); });
  const digest = sha256.create();
  try {
    live(); const unavailable = contentEligibility(version, format); if (unavailable) throw new MobileError('CONTENT_UNAVAILABLE', unavailable);
    const declared = contentFormat(version);
    if (!declared || declared.kind !== format.kind || declared.extension !== format.extension || declared.mime !== format.mime || declared.limit !== format.limit) throw bad();
    if (!isID(version.id) || !/^[a-f0-9]{64}$/.test(version.sha256)) throw bad();
    const total = size(version.size_bytes), grant = await transport.grant(version.id, scope); live();
    // The returned URL is deliberately never consumed. Only a validated permit
    // ID enters our same-origin relative JSON transport; no cloud credentials.
    if (!isID(grant.id) || grant.file_version_id !== version.id || grant.sha256 !== version.sha256 || grant.size_bytes !== version.size_bytes || grant.mode !== 'authenticated_proxy' || grant.method !== 'GET' || grant.disposition !== 'attachment' || grant.scan_state !== 'clean' || grant.required_headers.length !== 0 || size(grant.max_range_bytes) < CONTENT_CHUNK_BYTES) throw bad('文件授权与本次固定版本不符，已停止读取。');
    const expiry = Math.min(Date.parse(grant.expires_at), Date.parse(grant.permit_expires_at));
    let offset = 0;
    do {
      live(); if (!Number.isFinite(expiry) || expiry <= (input.now?.() ?? Date.now())) throw new MobileError('DOWNLOAD_EXPIRED', '本次读取授权已到期，请重新读取这个固定版本。');
      const length = Math.min(CONTENT_CHUNK_BYTES, total - offset);
      const chunk = await transport.chunk(grant.id, offset, length, version.sha256, scope); live();
      if (chunk.download_id !== grant.id || chunk.file_version_id !== version.id || chunk.offset !== String(offset) || chunk.length !== String(length) || chunk.total_size_bytes !== version.size_bytes || chunk.sha256 !== version.sha256 || !/^[a-f0-9]{64}$/.test(chunk.chunk_sha256)) throw bad();
      const bytes = decodeContentBase64(chunk.data_base64, length);
      if (hex(sha256(bytes)) !== chunk.chunk_sha256) throw bad();
      if (offset === 0) assertSignature(bytes, format, total);
      digest.update(bytes); await sink.write(bytes); live(); offset += length; input.progress?.(offset, total);
    } while (offset < total);
    if (hex(digest.digest()) !== version.sha256) throw bad('完整内容摘要未通过校验，临时内容已丢弃。');
    live(); const result = await sink.finish(); live(); return result;
  } catch (error) { await sink.abort(); throw error; }
  finally { digest.destroy(); release(); }
}
