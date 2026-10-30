import assert from 'node:assert/strict';
import { checkedComparison, comparisonEnd, comparisonMeaning, comparisonPath, type ComparisonSelection, type FileComparison } from '../src/file-comparison';
import type { components } from '../src/api';
type S = components['schemas'];
const id = (n: number) => '00000000-0000-4000-8000-' + n.toString().padStart(12, '0');
const hash = (n: number) => n.toString().repeat(64);
const time = '2026-10-29T15:00:00Z';
function fixture(): { selection: ComparisonSelection; page: FileComparison } {
  const file = { id: id(1), version: 4, state: 'active' } as S['File'];
  const base = { id: id(2), file_id: file.id, seq: 1, sha256: hash(1), size_bytes: '10', media_type: 'text/markdown', availability: 'available', scan_state: 'clean' } as S['FileVersion'];
  const target = { ...base, id: id(3), seq: 2, sha256: hash(2), size_bytes: '12' };
  const version = (v: S['FileVersion'], h: string) => ({ version_id: v.id, extractor: 'text-v1' as const, seq: v.seq, media_type: v.media_type, size_bytes: v.size_bytes, source_sha256: v.sha256, text_sha256: h, text_characters: 10, indexed_at: time });
  return { selection: { file, base, target, view: 'all' }, page: { file_id: file.id, file_version: 4, extractor: 'text-v1', base: version(base, hash(3)), target: version(target, hash(4)), source_equal: false, text_equal: false, algorithm: 'myers-v1', complete: true, view: 'all', base_lines: 2, target_lines: 2, added_lines: 1, removed_lines: 1, unchanged_lines: 1, comparison_sha256: hash(5), offset: 0, base_offset: 0, target_offset: 0, total_rows: 3,
    rows: [{ kind: 'equal', base_line: 1, target_line: 1, text: '香港🙂', newline: true, count: 1 }, { kind: 'delete', base_line: 2, target_line: null, text: '旧', newline: false, count: 1 }, { kind: 'insert', base_line: null, target_line: 2, text: '<script>新</script>', newline: true, count: 1 }], next_offset: null, as_of: time } };
}
type Case = readonly [string, () => void];
function rejects(name: string, change: (v: ReturnType<typeof fixture>) => void): Case {
  return [name, () => { const f = fixture(); change(f); assert.throws(() => checkedComparison(f.page, f.selection)); }];
}
export const comparisonCases: readonly Case[] = [
  ['full Unicode and final-newline evidence keeps exact selection', () => { const f = fixture(); assert.equal(checkedComparison(f.page, f.selection), f.page); assert.deepEqual(comparisonEnd(f.page), [2, 2]); assert.match(comparisonPath(f.selection), /base_version_id=.*&target_version_id=.*&view=all&offset=0&limit=60$/); }],
  ['read-only paging requires the exact first comparison hash', () => { const f = fixture(); assert.throws(() => comparisonPath(f.selection, 1)); assert.ok(comparisonPath(f.selection, 1, f.page).endsWith('&comparison_sha256=' + hash(5))); }],
  ['two pages account for all base and target coordinates', () => { const f = fixture(); const first = { ...f.page, rows: f.page.rows.slice(0, 2), next_offset: 2 }; checkedComparison(first, f.selection); const last = { ...f.page, offset: 2, base_offset: 2, target_offset: 1, rows: f.page.rows.slice(2) }; assert.equal(checkedComparison(last, f.selection, 2, first), last); }],
  ['normalized text equality does not claim original-byte equality', () => { const f = fixture(); f.page.target.text_sha256 = f.page.base.text_sha256; Object.assign(f.page, { text_equal: true, added_lines: 0, removed_lines: 0, unchanged_lines: 2, total_rows: 2, rows: [f.page.rows[0], { ...f.page.rows[0], base_line: 2, target_line: 2 }] }); checkedComparison(f.page, f.selection); assert.match(comparisonMeaning(f.page), /原件字节不同/); }],
  ['context skips are explicit and consume both complete line ranges', () => { const f = fixture(); f.selection.view = 'changes'; f.page.view = 'changes'; f.page.rows[0] = { kind: 'skip', base_line: 1, target_line: 1, text: '', newline: false, count: 1 }; checkedComparison(f.page, f.selection); }],
  ['Office and mixed formats bind both extraction identities', () => { const f = fixture(); f.selection.target.media_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'; f.page.target.media_type = f.selection.target.media_type; f.page.target.extractor = 'office-v1'; f.page.extractor = 'mixed'; checkedComparison(f.page, f.selection); f.page.target.extractor = 'text-v1'; assert.throws(() => checkedComparison(f.page, f.selection)); }],
  ['identical source bytes may have different text under different extraction identities', () => { const f = fixture(); f.selection.target.sha256 = f.selection.base.sha256; f.page.target.source_sha256 = f.page.base.source_sha256; f.page.source_equal = true; f.selection.target.media_type = 'application/vnd.openxmlformats-officedocument.presentationml.presentation'; f.page.target.media_type = f.selection.target.media_type; f.page.target.extractor = 'office-v1'; f.page.extractor = 'mixed'; checkedComparison(f.page, f.selection); assert.match(comparisonMeaning(f.page), /提取格式或方式不同/); }],
  rejects('rejects another file even with similar version labels', f => { f.page.file_id = id(8); }),
  rejects('rejects a changed file revision', f => { f.page.file_version++; }),
  rejects('rejects another exact base version', f => { f.page.base.version_id = id(9); }),
  rejects('rejects a changed original digest', f => { f.page.target.source_sha256 = hash(7); }),
  rejects('rejects incomplete comparison claims', f => { f.page.complete = false as true; }),
  rejects('rejects hidden rows in the full-text view', f => { f.page.rows[0].kind = 'skip'; f.page.rows[0].text = ''; f.page.rows[0].newline = false; }),
  rejects('rejects a missing final page line', f => { f.page.rows.pop(); f.page.total_rows--; }),
  rejects('rejects reversed or duplicate coordinates', f => { f.page.rows[1].base_line = 1; }),
  rejects('rejects a next-page pointer that skips result rows', f => { f.page.next_offset = 4; }),
  rejects('rejects inconsistent line totals', f => { f.page.added_lines++; }),
  rejects('rejects byte-equality claims with unequal original hashes', f => { f.page.source_equal = true; }),
  rejects('rejects changed bodies presented as equal text', f => { f.page.text_equal = true; }),
  rejects('rejects unscanned or waived selections', f => { f.selection.target.scan_state = 'waived'; }),
  ['a later page cannot change comparison identity or boundaries', () => { const f = fixture(); const first = { ...f.page, rows: f.page.rows.slice(0, 2), next_offset: 2 }; const last = { ...f.page, offset: 2, base_offset: 2, target_offset: 1, rows: f.page.rows.slice(2), comparison_sha256: hash(6) }; assert.throws(() => checkedComparison(last, f.selection, 2, first)); last.comparison_sha256 = first.comparison_sha256; last.target_offset = 0; assert.throws(() => checkedComparison(last, f.selection, 2, first)); }],
];
