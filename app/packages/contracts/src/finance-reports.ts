import type { components } from './api';
import { checkedFXValuation } from './finance-fx';

type S = components['schemas'];
export type ReportQuery = Required<Omit<S['FinancialReportQuery'], 'fx_book_id'>> & Pick<S['FinancialReportQuery'], 'fx_book_id'>;
export type FinancialReport = S['FinancialReport'];
export type ReportSource = S['FinancialReportSource'];
export type ReportExport = S['FinancialReportExport'];
export type ReportExportChunk = S['FinancialReportExportChunk'];
export type ReportOption = { id: string; value: string; label: string };
export const reportKinds = [{ value: 'incurred', label: '发生支出' }, { value: 'cash', label: '现金支出' }, { value: 'forecast', label: '订阅预测' }] as const;
export const exportStates: Record<ReportExport['state'], string> = { queued: '等待生成', running: '正在生成', retry_wait: '等待重试', succeeded: '已生成', dead: '生成失败', cancelled: '已取消', expired: '已过期' };
export const REPORT_EXPORT_BYTES = 16 << 20;
export const REPORT_CHUNK_BYTES = 512 << 10;
export const formatReportAmount = (value: string) => value.includes('.') ? value.replace(/0+$/, '').replace(/\.$/, '') : value;
const id = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/i.test(value);
const money = (value: unknown): value is string => typeof value === 'string' && /^-?(0|[1-9][0-9]*)(\.[0-9]{1,6})?$/.test(value);
const hash = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const invalid = (message = '报表资料与所选条件不一致，请重新读取。') => new Error(message);

export function monthWindow(month: string): { from: string; to: string } {
  if (!/^\d{4}-(0[1-9]|1[0-2])$/.test(month)) throw invalid('请选择有效月份。');
  const year = Number(month.slice(0, 4)), value = Number(month.slice(5));
  if (year < 1900 || year > 9998) throw invalid('月份需在1900至9998年之间。');
  return { from: month + '-01', to: value === 12 ? String(year + 1) + '-01-01' : String(year) + '-' + String(value + 1).padStart(2, '0') + '-01' };
}
export function currentReportQuery(kind: ReportQuery['kind'] = 'incurred', project: string | null = null): ReportQuery {
  const parts = new Intl.DateTimeFormat('en', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit' }).formatToParts(new Date());
  const month = parts.find(item => item.type === 'year')!.value + '-' + parts.find(item => item.type === 'month')!.value;
  return { kind, ...monthWindow(month), currency: '', category: '', q: '', project_id: project, company_only: false, supplier_id: null, owner_id: null };
}
export function validateReportQuery(query: ReportQuery): ReportQuery {
  if (!reportKinds.some(item => item.value === query.kind) || !/^\d{4}-\d\d-\d\d$/.test(query.from) || !/^\d{4}-\d\d-\d\d$/.test(query.to)) throw invalid('请选择报表类型和完整期间。');
  const from = Date.parse(query.from + 'T00:00:00Z'), to = Date.parse(query.to + 'T00:00:00Z');
  if (!Number.isFinite(from) || !Number.isFinite(to) || new Date(from).toISOString().slice(0, 10) !== query.from || new Date(to).toISOString().slice(0, 10) !== query.to || to <= from || to - from > 366 * 86400000) throw invalid('期间需为1至366个自然日，结束日不含在内。');
  if (!/^([A-Z]{3})?$/.test(query.currency) || [...query.category].length > 50 || [...query.q].length > 200 || /\0/.test(query.category + query.q) || query.project_id && query.company_only || typeof query.company_only !== 'boolean') throw invalid('请核对币种、类别、关键词与分摊范围。');
  for (const value of [query.project_id, query.supplier_id, query.owner_id, query.fx_book_id ?? null]) if (value !== null && !id(value)) throw invalid('请重新选择项目、供应方或经办／负责人。');
  return { ...query };
}
export function reportQueryKey(query: ReportQuery): string {
  return JSON.stringify([query.kind, query.from, query.to, query.currency, query.category, query.q, query.project_id, query.company_only, query.supplier_id, query.owner_id, query.fx_book_id ?? null]);
}
export function financialReportPath(query: ReportQuery, cursor = '', limit = 20): string {
  validateReportQuery(query);
  const params = new URLSearchParams({ kind: query.kind, from: query.from, to: query.to, limit: String(limit) });
  for (const name of ['currency', 'category', 'q', 'project_id', 'supplier_id', 'owner_id', 'fx_book_id'] as const) if (query[name]) params.set(name, query[name]!);
  if (query.company_only) params.set('company_only', 'true');
  if (cursor) params.set('cursor', cursor);
  return '/finance/reports?' + params;
}
export function reportSourcePath(source: ReportSource): string {
  return source.source_type === 'cost' ? '/costs/' + source.id : source.source_type === 'payment' ? '/payments/' + source.id : '/subscription-periods/' + source.id;
}
export function checkedFinancialReport(value: FinancialReport, query: ReportQuery): FinancialReport {
  if (!value || !value.filters || reportQueryKey(value.filters as ReportQuery) !== reportQueryKey(query) || value.scope !== 'authorized_records_only' || !value.summary || value.summary.kind !== query.kind || value.summary.from !== query.from || value.summary.to !== query.to || !Number.isFinite(Date.parse(value.summary.as_of)) || value.summary.base_currency !== null || value.summary.fx_coverage !== 'unconfigured' || !Array.isArray(value.summary.totals) || !Array.isArray(value.items) || value.items.length > 100 || value.next_cursor !== null && (typeof value.next_cursor !== 'string' || !value.next_cursor)) throw invalid();
  for (const total of value.summary.totals) {
    if (!/^[A-Z]{3}$/.test(total.currency) || query.currency && total.currency !== query.currency) throw invalid();
    for (const [key, amount] of Object.entries(total)) if (key.endsWith('_amount') && !money(amount)) throw invalid('报表金额不是精确十进制字符串。');
  }
  if (query.fx_book_id) checkedFXValuation(value.valuation!, query.fx_book_id);
  else if (value.valuation) throw invalid('报表返回了未选择的汇率版本。');
  const expected = query.kind === 'incurred' ? 'cost' : query.kind === 'cash' ? 'payment' : 'subscription_period';
  for (const item of value.items) {
    if (!id(item.id) || !id(item.owner_id) || item.supplier_id !== null && !id(item.supplier_id) || !id(item.resource_id) || !id(item.revision_id) || !Number.isSafeInteger(item.version) || item.version < 1 || item.source_type !== expected || expected !== 'subscription_period' && item.resource_id !== item.id || item.from < query.from || item.from >= query.to || item.currency !== null && (!/^[A-Z]{3}$/.test(item.currency) || query.currency && item.currency !== query.currency)) throw invalid();
    if (query.fx_book_id) { const fx = item.valuation; if (!fx || !/^\d{4}-\d\d-\d\d$/.test(fx.fx_date) || !['identity','converted','missing_rate','unconfirmed','unpriced'].includes(fx.status) || ['identity','converted'].includes(fx.status) !== (fx.base_amount !== null) || fx.base_amount !== null && !money(fx.base_amount) || item.state === 'draft' && fx.status !== 'unconfirmed' || fx.status === 'converted' && (fx.rate_index === null || fx.rate_source === null || fx.rate_date === null || fx.valid_from === null || fx.valid_to === null || fx.fx_date < fx.valid_from || fx.fx_date >= fx.valid_to) || fx.status === 'identity' && (item.currency !== value.valuation!.book.base_currency || fx.rate !== '1') || ['identity','converted'].includes(fx.status) && (!fx.rate || !/^(0|[1-9][0-9]*)(\.[0-9]{1,10})?$/.test(fx.rate))) throw invalid('折算明细缺少原始依据。'); }
    else if (item.valuation) throw invalid('明细返回了未选择的折算。');
    if (item.original_amount !== null && (!money(item.original_amount) || item.original_amount.startsWith('-')) || item.contribution_amount !== null && !money(item.contribution_amount) || item.state === 'draft' && item.contribution_amount !== null) throw invalid('未确认费用不能计入实际发生额。');
  }
  return value;
}
export function checkedReportExport(value: ReportExport, query?: ReportQuery, exportID?: string): ReportExport {
  if (!value || !id(value.id) || !id(value.job_id) || !value.filters || exportID && value.id !== exportID || query && reportQueryKey(value.filters as ReportQuery) !== reportQueryKey(query) || !Object.prototype.hasOwnProperty.call(exportStates, value.state) || !Number.isFinite(Date.parse(value.created_at)) || !Number.isFinite(Date.parse(value.expires_at))) throw invalid('导出回执不完整，请先查阅我的导出任务。');
  if (value.can_download !== (value.state === 'succeeded' && hash(value.csv_sha256)) || value.can_cancel !== ['queued', 'running', 'retry_wait'].includes(value.state)) throw invalid('导出状态不一致，请重新读取。');
  validateReportQuery(value.filters as ReportQuery);
  if (value.can_download && (!Number.isSafeInteger(value.csv_bytes) || !value.csv_bytes || value.csv_bytes > REPORT_EXPORT_BYTES || !Number.isSafeInteger(value.row_count) || value.row_count! < 0 || value.row_count! > 50000)) throw invalid('导出范围或摘要无效。');
  return value;
}

const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
function decodeBase64(value: string): Uint8Array {
  if (value.length > 699052 || value.length % 4 || !/^[A-Za-z0-9+/]*={0,2}$/.test(value)) throw invalid('导出分段编码无效。');
  const size = value.length / 4 * 3 - (value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0), out = new Uint8Array(size);
  let offset = 0;
  for (let i = 0; i < value.length; i += 4) {
    const a = alphabet.indexOf(value[i]), b = alphabet.indexOf(value[i + 1]), c = alphabet.indexOf(value[i + 2]), d = alphabet.indexOf(value[i + 3]);
    if (offset < size) out[offset++] = a << 2 | b >>> 4;
    if (offset < size) out[offset++] = (b & 15) << 4 | c >>> 2;
    if (offset < size) out[offset++] = (c & 3) << 6 | d;
    if (value[i + 2] === '=' && (b & 15) || value[i + 3] === '=' && c >= 0 && (c & 3)) throw invalid('导出分段编码不是规范Base64。');
  }
  return out;
}
export async function readReportExportBytes(record: ReportExport, fetchChunk: (offset: number, length: number) => Promise<ReportExportChunk>, sha256: (bytes: Uint8Array) => Promise<string> | string, checkActive: () => void): Promise<Uint8Array> {
  checkedReportExport(record);
  if (!record.can_download) throw invalid('报表尚未生成或已过期。');
  const result = new Uint8Array(record.csv_bytes!);
  try {
    for (let offset = 0; offset < result.length;) {
      checkActive();
      const length = Math.min(REPORT_CHUNK_BYTES, result.length - offset), chunk = await fetchChunk(offset, length);
      checkActive();
      if (chunk.export_id !== record.id || chunk.sha256 !== record.csv_sha256 || chunk.total_size_bytes !== String(result.length) || chunk.offset !== String(offset) || chunk.length !== String(length) || !hash(chunk.chunk_sha256) || chunk.filename !== 'finance-' + record.filters.kind + '-' + record.id + '.csv') throw invalid('导出分段与当前任务不一致。');
      const bytes = decodeBase64(chunk.data_base64);
      try {
        if (bytes.length !== length || await sha256(bytes) !== chunk.chunk_sha256) throw invalid('导出分段校验失败。');
        checkActive(); result.set(bytes, offset); offset += length;
      } finally { bytes.fill(0); }
    }
    checkActive();
    if (await sha256(result) !== record.csv_sha256) throw invalid('完整CSV摘要校验失败。');
    checkActive();
    return result;
  } catch (error) { result.fill(0); throw error; }
}
