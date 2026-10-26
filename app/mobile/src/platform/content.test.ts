import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import test from 'node:test';
import { contentEligibility, contentFormat, CONTENT_CHUNK_BYTES, decodeContentBase64, decodeContentText, readFixedContent, type ContentSink, type ContentTransport } from './content';
import { Scope, type Schemas } from './session';

const versionID = '11111111-2222-4333-8444-555555555555', permitID = 'aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee';
const hash = (bytes: Uint8Array) => createHash('sha256').update(bytes).digest('hex');
function harness(payload = Buffer.from('固定版本\n<svg onload="doNotRun()">不是标记</svg>')) {
  const version = { id: versionID, file_id: permitID, size_bytes: String(payload.length), sha256: hash(payload), media_type: 'text/plain', original_name: 'fixed.txt', availability: 'available', scan_state: 'clean' } as Schemas['FileVersion'];
  const grant = { id: permitID, file_version_id: version.id, mode: 'authenticated_proxy', method: 'GET', url: 'https://unused.invalid/must-never-be-fetched', required_headers: [], disposition: 'attachment', scan_state: 'clean', size_bytes: version.size_bytes, sha256: version.sha256, max_range_bytes: '33554432', expires_at: '2030-01-01T00:00:00Z', permit_expires_at: '2030-01-01T00:00:00Z', warnings: [], filename: 'fixed.txt' } as Schemas['DownloadResult'];
  const calls: { offset: number; length: number }[] = [], writes: Uint8Array[] = [];
  let finished = false, aborted = 0, grants = 0;
  const scope = new Scope();
  const transport: ContentTransport = {
    async grant(id, own) { assert.equal(id, version.id); assert.equal(own, scope); grants++; return grant; },
    async chunk(id, offset, length, sha, own) {
      assert.equal(id, grant.id); assert.equal(sha, version.sha256); assert.equal(own, scope); calls.push({ offset, length });
      const bytes = payload.subarray(offset, offset + length);
      return { download_id: grant.id, file_version_id: version.id, offset: String(offset), length: String(length), total_size_bytes: version.size_bytes, sha256: version.sha256, chunk_sha256: hash(bytes), data_base64: bytes.toString('base64') };
    },
  };
  const sink: ContentSink<Buffer> = { async write(bytes) { writes.push(bytes); }, async finish() { finished = true; return Buffer.concat(writes); }, async abort() { aborted++; writes.length = 0; } };
  const read = (progress?: (received: number, total: number) => void, now?: () => number) => readFixedContent({ version, format: contentFormat(version)!, scope, sink, transport, progress, now });
  return { version, grant, calls, writes, scope, transport, sink, read, stats: () => ({ finished, aborted, grants }) };
}
function large() { const bytes = Buffer.alloc(CONTENT_CHUNK_BYTES + 123, 72); Buffer.from('%PDF-1.7\n').copy(bytes); const h = harness(bytes); h.version.media_type = 'application/pdf'; h.version.original_name = 'fixed.pdf'; return { h, bytes }; }

test('fixed content transfers sequential bounded ranges and validates the full SHA with an independent oracle', async () => {
  const { h, bytes } = large(), progress: number[] = [];
  assert.deepEqual(await h.read(n => progress.push(n)), bytes);
  assert.deepEqual(h.calls, [{ offset: 0, length: CONTENT_CHUNK_BYTES }, { offset: CONTENT_CHUNK_BYTES, length: 123 }]);
  assert.deepEqual(progress, [CONTENT_CHUNK_BYTES, bytes.length]); assert.equal(h.stats().finished, true);
});
test('an empty text version still authenticates one empty range and validates the empty SHA', async () => {
  const h = harness(Buffer.alloc(0)); assert.equal((await h.read()).length, 0); assert.deepEqual(h.calls, [{ offset: 0, length: 0 }]);
});
test('a permit cannot change the fixed version, hash, size, source mode or required headers', async () => {
  for (const altered of [{ file_version_id: permitID }, { sha256: '0'.repeat(64) }, { size_bytes: '0' }, { mode: 'signed_url' }, { required_headers: [{ name: 'Authorization', value: 'forbidden' }] }, { disposition: 'inline' }, { scan_state: 'waived' }]) {
    const h = harness(); Object.assign(h.grant, altered); await assert.rejects(h.read(), { code: 'CONTENT_INTEGRITY_FAILED' }); assert.equal(h.calls.length, 0); assert.equal(h.stats().finished, false);
  }
});
test('range identity, canonical base64 and each chunk digest are verified before any sink write', async () => {
  for (const altered of [{ offset: '00' }, { length: '0' }, { total_size_bytes: '9000' }, { download_id: versionID }, { file_version_id: permitID }, { sha256: '0'.repeat(64) }, { chunk_sha256: '0'.repeat(64) }, { data_base64: 'AAAA' }]) {
    const h = harness(), original = h.transport.chunk;
    h.transport.chunk = async (...args) => Object.assign(await original(...args), altered);
    await assert.rejects(h.read(), { code: 'CONTENT_INTEGRITY_FAILED' }); assert.equal(h.writes.length, 0); assert.equal(h.stats().finished, false);
  }
});
test('self-consistent chunk digests do not bypass a mismatching full-version digest', async () => {
  const h = harness(); h.version.sha256 = h.grant.sha256 = '1'.repeat(64);
  await assert.rejects(h.read(), { code: 'CONTENT_INTEGRITY_FAILED' }); assert.equal(h.stats().finished, false); assert.equal(h.writes.length, 0);
});
test('closing the scope while a response is pending discards late bytes and aborts the sink', async () => {
  const h = harness(), original = h.transport.chunk; let resolve!: () => void;
  const held = new Promise<void>(yes => { resolve = yes; });
  h.transport.chunk = async (...args) => { await held; return original(...args); };
  const reading = h.read(), rejection = assert.rejects(reading, { code: 'REQUEST_CANCELLED' });
  await new Promise<void>(yes => setImmediate(yes)); h.scope.close(); resolve(); await rejection;
  assert.equal(h.stats().finished, false); assert.equal(h.writes.length, 0); assert.ok(h.stats().aborted > 0);
});
test('expiry is checked before each range and never replaces a file version or automatically grants again', async () => {
  const { h } = large(); let now = Date.parse('2026-10-24T00:00:00Z');
  await assert.rejects(h.read(() => { now = Date.parse('2031-01-01T00:00:00Z'); }, () => now), { code: 'DOWNLOAD_EXPIRED' });
  assert.equal(h.calls.length, 1); assert.equal(h.stats().grants, 1); assert.equal(h.stats().finished, false); assert.equal(h.writes.length, 0);
});
test('a failed sink never produces a completed artifact', async () => {
  const h = harness(); h.sink.write = async () => { throw new Error('owned disk full'); };
  await assert.rejects(h.read(), /disk full/); assert.equal(h.stats().finished, false); assert.ok(h.stats().aborted > 0);
});
test('format signatures reject an active-document disguise before writing or exposing content', async () => {
  const h = harness(Buffer.from('<html>not a PDF</html>')); h.version.media_type = 'application/pdf';
  await assert.rejects(h.read(), { code: 'CONTENT_INTEGRITY_FAILED' }); assert.equal(h.writes.length, 0); assert.equal(h.stats().finished, false);
});
test('unavailable, waived, unsupported and excessive versions do not create a download permit', async () => {
  for (const altered of [{ availability: 'missing' }, { scan_state: 'waived' }, { scan_state: 'pending' }, { size_bytes: '120000000000' }, { size_bytes: '01' }]) {
    const h = harness(); Object.assign(h.version, altered); await assert.rejects(h.read(), { code: 'CONTENT_UNAVAILABLE' }); assert.equal(h.stats().grants, 0);
  }
  assert.equal(contentFormat({ media_type: 'image/svg+xml', original_name: 'unsafe.svg' }), null);
  assert.equal(contentFormat({ media_type: 'text/html', original_name: 'unsafe.html' }), null);
  assert.ok(contentEligibility({ availability: 'available', scan_state: 'clean', size_bytes: '12' }, null));
});
test('base64 decoder rejects noncanonical padding, whitespace and length mismatches', () => {
  for (let length = 0; length < 9; length++) { const bytes = Buffer.alloc(length, 203); assert.deepEqual(decodeContentBase64(bytes.toString('base64'), length), new Uint8Array(bytes)); }
  for (const [value, length] of [['Zh==', 1], ['Zm9=', 2], ['Zg=\n', 1], ['Zg==AAAA', 1], ['AA=A', 3], ['====', 1]] as const) assert.throws(() => decodeContentBase64(value, length), { code: 'CONTENT_INTEGRITY_FAILED' });
});
test('portable UTF-8 decoding preserves Chinese, emoji and literal markup and rejects malformed sequences', () => {
  const text = '香港🙂\n<script>只是原文</script>\r\n'; assert.equal(decodeContentText(Buffer.from('\uFEFF' + text)), text);
  for (const bytes of [[0xc0, 0xaf], [0xe0, 0x80, 0x80], [0xed, 0xa0, 0x80], [0xf4, 0x90, 0x80, 0x80], [0xe4, 0xb8], [0], [0x80]]) assert.throws(() => decodeContentText(Uint8Array.from(bytes)), { code: 'CONTENT_INTEGRITY_FAILED' });
});
