import type { components } from './api';
type S = components['schemas'];
export type SearchHit = S['SearchHit'];
export type FileText = S['FileText'];
export type SearchPage = { items: SearchHit[]; next_cursor: string | null };
export const resourceNames = { project: '项目', task: '任务', post: '知识记录', file: '文件', folder: '目录' } as const;
export type SearchType = '' | keyof typeof resourceNames;
export const officeTextNotice = 'Office 正文为文字提取：Excel 数值保留原值、公式不计算且缓存可能过期；PowerPoint 保留形状顺序与讲者备注。包含隐藏内容，版式及图像请查原件。';
export const docxTextNotice = 'Word 正文为文字提取：含表格、显式引用的页眉页脚、脚注尾注、批注及文本框。插入、删除、移动均保留标记，未接受修订；字段不计算，缓存可能过期。包含隐藏文字，样式、分页、合并单元格及图像请核对原件。';
export const documentTextNotice = 'PDF逐页保留内嵌文字及OCR结果；图片采用OCR识别。OCR可能误读、遗漏或转换简繁体，金额、日期、合同条款均须核对原件；不还原版式，也不自动记账。';
export const extractionNotice = (extractor: string) => extractor === 'document-ocr-v1' ? documentTextNotice : extractor === 'docx-v2' ? docxTextNotice : extractor === 'mixed' ? docxTextNotice + ' ' + officeTextNotice + ' ' + documentTextNotice : officeTextNotice;
export const nativePageNotice = (evidence: S['NativeTextEvidence'] | undefined) => evidence ? '完整处理 ' + evidence.page_count + ' 页 · ' + evidence.ocr_page_count + ' 页含OCR，需核对原件。' : '';
export const textStates: Record<FileText['state'], string> = { pending: '正文待提取', ready: '正文已收录', unsupported: '此格式或大小暂未支持', blocked: '来源已阻止读取', failed: '正文提取失败' };
const id = (x: unknown): x is string => typeof x === 'string' && /^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/i.test(x);
const hash = (x: unknown): x is string => typeof x === 'string' && /^[a-f0-9]{64}$/.test(x);
const date = (x: unknown) => typeof x === 'string' && Number.isFinite(Date.parse(x));
const own = (o: object, key: unknown) => typeof key === 'string' && Object.prototype.hasOwnProperty.call(o, key);
const invalid = () => new Error('搜索或正文资料与所选版本不一致，请刷新后重试。');
export function validNativeEvidence(extractor: string, state: string, evidence: unknown): boolean {
  if (extractor !== 'document-ocr-v1' || state !== 'ready') return evidence === undefined;
  const e = evidence as S['NativeTextEvidence'];
  return !!e && hash(e.engine_sha256) && Number.isSafeInteger(e.page_count) && e.page_count >= 1 && e.page_count <= 20 && e.ocr_page_count === e.page_count;
}
export function searchPath(q: string, type: SearchType = '', cursor = '', company = false) {
  q = q.trim(); if ([...q].length < 1 || [...q].length > 200 || q.includes('\0') || type && !own(resourceNames, type)) throw invalid();
  return '/search?q=' + encodeURIComponent(q) + '&limit=20' + (type ? '&type=' + type : '') + (company ? '&company_only=true' : '') + (cursor ? '&cursor=' + encodeURIComponent(cursor) : '');
}
export function checkedSearch(value: unknown): SearchPage {
  const p = value as SearchPage;
  if (!p || !Array.isArray(p.items) || p.items.length > 20 || p.next_cursor !== null && (typeof p.next_cursor !== 'string' || !p.next_cursor || p.next_cursor.length > 2048)) throw invalid();
  const seen = new Set<string>();
  for (const h of p.items) {
    if (!h || !h.resource || !own(resourceNames, h.resource.type) || !id(h.resource.id) || h.resource.version_id != null && !id(h.resource.version_id) || typeof h.title !== 'string' || !h.title || [...h.title].length > 200 || typeof h.snippet !== 'string' || [...h.snippet].length > 240 || !date(h.updated_at) || h.project_id !== null && !id(h.project_id) || !Array.isArray(h.match_types) || !h.match_types.length || h.match_types.some(x => !['name', 'body', 'tags'].includes(x)) || new Set(h.match_types).size !== h.match_types.length || h.indexed_at !== null && !date(h.indexed_at)) throw invalid();
    if (h.resource.type === 'file' ? !h.text_state || !own(textStates, h.text_state) || h.text_state === 'ready' && (!id(h.resource.version_id) || !date(h.indexed_at)) : h.text_state !== null || h.indexed_at !== null) throw invalid();
    if (h.text_extractor !== undefined && (h.resource.type !== 'file' || !['text-v1', 'office-v1', 'docx-v2', 'document-ocr-v1'].includes(h.text_extractor))) throw invalid();
    const key = h.resource.type + '/' + h.resource.id; if (seen.has(key)) throw invalid(); seen.add(key);
  }
  return p;
}
export function fileTextPath(file: string, version: string, offset = 0) {
  if (!id(file) || !id(version) || !Number.isSafeInteger(offset) || offset < 0 || offset > (2 << 20)) throw invalid();
  return '/files/' + file + '/versions/' + version + '/text?limit=4000&offset=' + offset;
}
export function checkedFileText(value: unknown, file: string, version: string, offset: number): FileText {
  const v = value as FileText;
  if (!v || v.file_id !== file || v.version_id !== version || !['text-v1', 'office-v1', 'docx-v2', 'document-ocr-v1'].includes(v.extractor) || !validNativeEvidence(v.extractor, v.state, v.native_evidence) || !own(textStates, v.state) || !hash(v.source_sha256) || v.offset !== offset || typeof v.text !== 'string' || [...v.text].length > 4000 || v.indexed_at !== null && !date(v.indexed_at) || v.error_code !== null && (typeof v.error_code !== 'string' || !/^FILE_TEXT_[A-Z_]{1,64}$/.test(v.error_code))) throw invalid();
  if (v.state === 'ready') {
    if (!hash(v.text_sha256) || !date(v.indexed_at) || v.error_code !== null || !Number.isSafeInteger(v.total_characters) || v.total_characters! < offset || v.total_characters! > (2 << 20)) throw invalid();
    const end = offset + [...v.text].length;
    if (end > v.total_characters! || (end < v.total_characters! ? v.next_offset !== end || end === offset : v.next_offset !== null)) throw invalid();
  } else if (v.text !== '' || v.text_sha256 !== null || v.total_characters !== null || v.next_offset !== null) throw invalid();
  return v;
}
