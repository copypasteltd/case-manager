import type { components } from './api';

type S = components['schemas'];
export type FXBook = S['FinanceFXBook'];
export type FXPolicy = S['FinanceFXPolicy'];
export type FXContent = S['FinanceFXContent'];
export type FXValuation = S['FinancialReportValuation'];
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
const count = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) >= 0;
const decimal = (v: unknown): v is string => typeof v === 'string' && /^-?(0|[1-9][0-9]*)(\.[0-9]{1,6})?$/.test(v);
const ratePattern = /^(0|[1-9][0-9]{0,9})(\.[0-9]{1,10})?$/;
const bad = () => new Error('汇率版本或折算回执不完整，请重新读取。');
function day(v: string): boolean { return /^\d{4}-\d\d-\d\d$/.test(v) && Number.isFinite(Date.parse(v + 'T00:00:00Z')) && new Date(v + 'T00:00:00Z').toISOString().slice(0, 10) === v; }
function units(value: string): bigint { const [a,b=''] = value.replace(/^-/, '').split('.'); const v = BigInt(a) * 1000000n + BigInt(b.padEnd(6, '0')); return value.startsWith('-') ? -v : v; }
export function checkedFXBook(book: FXBook, full = false): FXBook {
  if (!book || !uuid(book.id) || !uuid(book.creator_id) || !Number.isSafeInteger(book.revision) || book.revision < 1 || !/^[A-Z]{3}$/.test(book.base_currency) || !count(book.rate_count) || book.rate_count > 2000 || !/^[a-f0-9]{64}$/.test(book.content_sha256) || !book.title?.trim() || !book.reason?.trim() || !Number.isFinite(Date.parse(book.created_at))) throw bad();
  if (full && !book.content) throw bad();
  if (book.content) { const v = checkedFXContent(book.content); if (v.title !== book.title || v.base_currency !== book.base_currency || v.rates.length !== book.rate_count) throw bad(); }
  return book;
}
export function checkedFXPolicy(v: FXPolicy): FXPolicy {
  if (!v || !count(v.version) || (v.version === 0) !== (v.current === null)) throw bad();
  if (v.current && checkedFXBook(v.current).revision !== v.version) throw bad();
  return v;
}
export function checkedFXContent(v: FXContent): FXContent {
  if (!v || !v.title?.trim() || [...v.title].length > 200 || !/^[A-Z]{3}$/.test(v.base_currency) || v.method !== 'manual_interval' || v.rounding !== 'scope_line_half_up' || !Array.isArray(v.rates) || v.rates.length > 2000) throw new Error('请填写标题、基础币种和最多2000条人工汇率。');
  const previous = new Map<string,string>();
  for (const r of [...v.rates].sort((a,b) => a.currency.localeCompare(b.currency) || a.valid_from.localeCompare(b.valid_from))) {
    if (!/^[A-Z]{3}$/.test(r.currency) || r.currency === v.base_currency || !day(r.valid_from) || !day(r.valid_to) || !day(r.rate_date) || r.valid_to <= r.valid_from || Date.parse(r.valid_to) - Date.parse(r.valid_from) > 366 * 86400000 || !ratePattern.test(r.rate) || !/[1-9]/.test(r.rate) || !r.source?.trim() || [...r.source].length > 200 || (previous.get(r.currency) ?? '') > r.valid_from) throw new Error('汇率须为正十进制字符串；每行填写币种、起始日、结束日、报价日、汇率、来源，同币种区间不可重叠。');
    previous.set(r.currency, r.valid_to);
  }
  return v;
}
// TSV avoids ambiguous source citations containing commas. Never parses rates
// through Number, so ten-place quote precision survives every client.
export function fxContentFromText(title: string, baseCurrency: string, text: string): FXContent {
  const lines = text.split(/\r?\n/).filter(v => v.trim());
  const rates = lines.map((line,index) => { const values = line.split('\t').map(v => v.trim()); if (values.length !== 6) throw new Error('第 ' + (index + 1) + ' 行需使用 Tab 分隔6列。'); const [currency,valid_from,valid_to,rate_date,rate,source] = values; return { currency,valid_from,valid_to,rate_date,rate,source }; });
  return checkedFXContent({ title: title.trim(), base_currency: baseCurrency.trim().toUpperCase(), method: 'manual_interval', rounding: 'scope_line_half_up', rates });
}
export const fxRatesText = (v: FXContent) => v.rates.map(r => [r.currency,r.valid_from,r.valid_to,r.rate_date,r.rate,r.source].join('\t')).join('\n');
export const fxProjectQuery = (project?: string | null) => project ? '?project_id=' + encodeURIComponent(project) : '';
export function checkedFXValuation(v: FXValuation, bookID: string): FXValuation {
  if (!v || checkedFXBook(v.book).id !== bookID || !count(v.exponent) || v.exponent > 6 || v.method !== 'manual_interval' || v.rounding !== 'scope_line_half_up' || !decimal(v.covered_amount) || v.total_amount !== null && !decimal(v.total_amount) || !Array.isArray(v.currencies)) throw bad();
  const keys = ['covered_count','missing_rate_count','unpriced_count','unconfirmed_count'] as const;
  if (keys.some(key => !count(v[key]))) throw bad();
  const totals = [0,0,0,0], currencies = new Set<string | null>(); let sum = 0n;
  for (const group of v.currencies) {
    if (group.currency !== null && !/^[A-Z]{3}$/.test(group.currency) || currencies.has(group.currency) || !decimal(group.covered_amount) || !decimal(group.missing_native_amount)) throw bad();
    currencies.add(group.currency); keys.forEach((key,i) => { if (!count(group[key])) throw bad(); totals[i] += group[key]; }); sum += units(group.covered_amount);
  }
  if (typeof v.source_coverage_complete !== 'boolean') throw bad();
  const complete = v.missing_rate_count + v.unpriced_count === 0 && v.source_coverage_complete;
  if (keys.some((key,i) => totals[i] !== v[key]) || sum !== units(v.covered_amount) || v.coverage !== (complete ? 'complete' : v.covered_count ? 'partial' : 'none') || complete !== (v.total_amount !== null) || v.total_amount !== null && units(v.total_amount) !== sum) throw bad();
  return v;
}
export const fxStatus = { identity: '基础币种', converted: '已折算', missing_rate: '缺少汇率', unconfirmed: '草稿未计入', unpriced: '缺少原币价格' };
