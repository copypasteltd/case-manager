import type { components } from './api';

type S = components['schemas'];
export type FileComparison = S['FileTextComparison'];
export type ComparisonView = 'changes' | 'all';
export type ComparisonSelection = { file: S['File']; base: S['FileVersion']; target: S['FileVersion']; view: ComparisonView };
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(v);
const hash = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
const count = (v: unknown, max = 80000): v is number => Number.isSafeInteger(v) && Number(v) >= 0 && Number(v) <= max;
const positive = (v: unknown): v is number => Number.isSafeInteger(v) && Number(v) > 0;
const invalid = () => new Error('版本比较依据、分页或完整内容回执不一致，请刷新文件后从第一页重新核对。');

export function comparisonEligible(version: S['FileVersion']): boolean {
  return version.availability === 'available' && version.scan_state === 'clean' && /^(text\/(plain|markdown|csv)|application\/json|application\/vnd\.openxmlformats-officedocument\.wordprocessingml\.document)(;|$)/i.test(version.media_type);
}

function selectionValid(s: ComparisonSelection): void {
  if (!s || !uuid(s.file.id) || !positive(s.file.version) || s.file.state !== 'active' || !['changes', 'all'].includes(s.view)) throw invalid();
  for (const v of [s.base, s.target]) if (!v || !uuid(v.id) || v.file_id !== s.file.id || !positive(v.seq) || !hash(v.sha256) || !/^[1-9][0-9]*$/.test(v.size_bytes) || !comparisonEligible(v)) throw new Error('请选择同一当前文件中扫描通过的文本或DOCX正式版本；提取状态由服务器进一步核对。');
}

export function comparisonPath(s: ComparisonSelection, offset = 0, previous?: FileComparison): string {
  selectionValid(s);
  if (!count(offset) || offset > 0 && !previous || previous && (!hash(previous.comparison_sha256) || previous.file_id !== s.file.id || previous.base.version_id !== s.base.id || previous.target.version_id !== s.target.id || previous.view !== s.view)) throw invalid();
  const query = 'base_version_id=' + s.base.id + '&target_version_id=' + s.target.id + '&view=' + s.view + '&offset=' + offset + '&limit=60';
  return '/files/' + s.file.id + '/text-comparison?' + query + (previous ? '&comparison_sha256=' + previous.comparison_sha256 : '');
}

export function comparisonEnd(v: FileComparison): [number, number] {
  let base = v.base_offset, target = v.target_offset;
  for (const row of v.rows) { if (row.base_line !== null) base += row.count; if (row.target_line !== null) target += row.count; }
  return [base, target];
}

export function checkedComparison(v: FileComparison, s: ComparisonSelection, offset = 0, previous?: FileComparison): FileComparison {
  selectionValid(s);
  if (!v || v.file_id !== s.file.id || v.file_version !== s.file.version || v.view !== s.view || v.extractor !== 'text-v1' || v.complete !== true || !hash(v.comparison_sha256) || !['myers-v1', 'bounded-replacement-v1'].includes(v.algorithm) || !Number.isFinite(Date.parse(v.as_of)) || !count(v.offset) || v.offset !== offset || !count(v.total_rows) || !count(v.base_lines, 40000) || !count(v.target_lines, 40000) || !count(v.added_lines) || !count(v.removed_lines) || !count(v.unchanged_lines) || v.base_lines !== v.removed_lines + v.unchanged_lines || v.target_lines !== v.added_lines + v.unchanged_lines || !count(v.base_offset, v.base_lines) || !count(v.target_offset, v.target_lines) || offset === 0 && (v.base_offset !== 0 || v.target_offset !== 0) || !Array.isArray(v.rows) || v.rows.length > 60 || v.offset + v.rows.length > v.total_rows || v.total_rows > 0 && v.offset < v.total_rows && v.rows.length === 0) throw invalid();
  for (const [actual, selected] of [[v.base, s.base], [v.target, s.target]] as const) if (!actual || actual.version_id !== selected.id || actual.seq !== selected.seq || actual.source_sha256 !== selected.sha256 || actual.size_bytes !== selected.size_bytes || actual.media_type !== selected.media_type || !hash(actual.text_sha256) || !count(actual.text_characters, 2097152) || !Number.isFinite(Date.parse(actual.indexed_at))) throw invalid();
  if (v.source_equal !== (v.base.source_sha256 === v.target.source_sha256) || v.text_equal !== (v.base.text_sha256 === v.target.text_sha256) || v.source_equal && !v.text_equal || (v.added_lines + v.removed_lines === 0) !== v.text_equal) throw invalid();
  if (previous && (previous.comparison_sha256 !== v.comparison_sha256 || previous.total_rows !== v.total_rows || previous.file_version !== v.file_version || previous.algorithm !== v.algorithm)) throw invalid();
  if (previous?.next_offset === offset) { const end = comparisonEnd(previous); if (end[0] !== v.base_offset || end[1] !== v.target_offset) throw invalid(); }
  let base = v.base_offset, target = v.target_offset;
  for (const row of v.rows) {
    if (!row || !['equal', 'delete', 'insert', 'skip'].includes(row.kind) || typeof row.text !== 'string' || row.text.includes('\n') || typeof row.newline !== 'boolean' || !positive(row.count) || row.count > 40000 || (row.kind === 'skip' ? s.view !== 'changes' || row.text !== '' || row.newline : row.count !== 1)) throw invalid();
    if (row.kind === 'insert') { if (row.base_line !== null) throw invalid(); } else { if (row.base_line !== base + 1) throw invalid(); base += row.count; }
    if (row.kind === 'delete') { if (row.target_line !== null) throw invalid(); } else { if (row.target_line !== target + 1) throw invalid(); target += row.count; }
    if (base > v.base_lines || target > v.target_lines) throw invalid();
  }
  const end = v.offset + v.rows.length;
  if (v.next_offset !== (end < v.total_rows ? end : null) || v.next_offset === null && (base !== v.base_lines || target !== v.target_lines)) throw invalid();
  return v;
}

export function comparisonMeaning(v: FileComparison): string {
  if (v.source_equal) return '两个原件字节摘要相同，提取正文也相同。';
  if (v.text_equal) return '提取正文相同，但原件字节不同；BOM、换行、排版或其他原件信息可能不同。';
  return '提取正文存在差异；下方比较不包含原件排版、图片、批注等非正文信息。';
}
