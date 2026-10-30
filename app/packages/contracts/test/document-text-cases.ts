import assert from 'node:assert/strict';
import { checkedFileText, checkedSearch, documentTextNotice, nativePageNotice, type FileText } from '../src/search';
import { extractorFor, comparisonEligible } from '../src/file-comparison';
import type { components } from '../src/api';
const id = '10000000-0000-4000-8000-000000000001', version = '10000000-0000-4000-8000-000000000002', time = '2026-10-31T00:00:00Z';
const native = () => ({ engine_sha256: 'c'.repeat(64), page_count: 2, ocr_page_count: 2 });
const text = (): FileText => ({ file_id: id, version_id: version, extractor: 'document-ocr-v1', state: 'ready', source_sha256: 'a'.repeat(64), text_sha256: 'b'.repeat(64), indexed_at: time, error_code: null, total_characters: 2, offset: 0, text: '合同', next_offset: null, native_evidence: native() });
type Case = readonly [string, () => void];
export const documentTextCases: readonly Case[] = [
 ['OCR preview requires complete exact engine/page evidence', () => { const v = text(); assert.equal(checkedFileText(v, id, version, 0), v); assert.match(nativePageNotice(v.native_evidence), /完整处理 2 页/); }],
 ['missing native evidence cannot present ready plaintext', () => { const v = text(); delete v.native_evidence; assert.throws(() => checkedFileText(v, id, version, 0)); }],
 ['partial or excessive page evidence is rejected', () => { for (const p of [0, 21, 1.5, NaN]) { const v = text(); v.native_evidence!.page_count = p; assert.throws(() => checkedFileText(v, id, version, 0)); } }],
 ['missing OCR pages and altered engine hashes are rejected', () => { for (const e of [{ ...native(), ocr_page_count: 1 }, { ...native(), engine_sha256: 'invented' }]) assert.throws(() => checkedFileText({ ...text(), native_evidence: e }, id, version, 0)); }],
 ['legacy extraction cannot borrow native page evidence', () => { for (const extractor of ['text-v1', 'office-v1', 'docx-v2'] as const) assert.throws(() => checkedFileText({ ...text(), extractor }, id, version, 0)); }],
 ['pending native work exposes no completed proof or partial text', () => { const v: FileText = { ...text(), state: 'pending', indexed_at: null, text: '', text_sha256: null, total_characters: null, native_evidence: undefined }; assert.equal(checkedFileText(v, id, version, 0).state, 'pending'); assert.throws(() => checkedFileText({ ...v, native_evidence: native() }, id, version, 0)); }],
 ['PDF PNG JPEG map to the same bounded document pipeline', () => { for (const media of ['application/pdf', 'image/png', 'IMAGE/JPEG; charset=binary']) { assert.equal(extractorFor(media), 'document-ocr-v1'); assert.equal(comparisonEligible({ media_type: media, availability: 'available', scan_state: 'clean' } as components['schemas']['FileVersion']), true); } }],
 ['OCR limits stay visible in product prose', () => { for (const phrase of ['核对原件', '简繁体', '不自动记账']) assert.ok(documentTextNotice.includes(phrase)); }],
 ['search discloses OCR identity without accepting unrelated resources', () => { const hit = { resource: { type: 'file', id, version_id: version }, title: '扫描.pdf', snippet: '合同', updated_at: time, project_id: null, match_types: ['body'], text_state: 'ready', indexed_at: time, text_extractor: 'document-ocr-v1' }; assert.equal(checkedSearch({ items: [hit], next_cursor: null }).items[0].text_extractor, 'document-ocr-v1'); for (const change of [{ text_extractor: 'unknown' }, { resource: { type: 'post', id }, text_state: null, indexed_at: null }]) assert.throws(() => checkedSearch({ items: [{ ...hit, ...change }], next_cursor: null })); }],
];
