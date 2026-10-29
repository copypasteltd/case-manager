import type { components } from './api';
import { decodeBase64 } from './finance-reports';

type S = components['schemas'];
export type AuditFilter = S['AuditFilter'];
export type AuditEvent = S['AuditEvent'];
export type AuditPage = S['AuditEventPage'];
export type AuditExport = S['AuditExport'];
export type AuditChunk = S['AuditExportChunk'];
export const AUDIT_BYTES = 8 << 20;
export const AUDIT_CHUNK = 512 << 10;
export const auditExportStates: Record<AuditExport['state'], string> = { ready: '可下载', expired: '已过期', authorization_changed: '权限依据已变化' };
export const auditFields = ['actor_id', 'executor_id', 'channel', 'action', 'result', 'resource_type', 'resource_id', 'request_id', 'run_id', 'job_id'] as const;
export const auditLabels: Record<typeof auditFields[number], string> = { actor_id: '操作者标识', executor_id: '执行者标识', channel: '来源渠道', action: '动作', result: '结果', resource_type: '资源类型', resource_id: '资源标识', request_id: '请求编号', run_id: '秘书任务标识', job_id: '后台任务标识' };
const identifiers = new Set<string>(['actor_id', 'executor_id', 'resource_id', 'run_id', 'job_id']);
const id = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/i.test(v);
const hash = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
const bad = (message = '审计资料与当前条件不一致，请重新读取。') => new Error(message);

export function validateAuditFilter(f: AuditFilter): AuditFilter {
  const from = Date.parse(f?.from), to = Date.parse(f?.to);
  if (!Number.isFinite(from) || !Number.isFinite(to) || !/^\d{4}-\d\d-\d\dT/.test(f.from) || !/T.*(?:Z|[+-]\d\d:\d\d)$/.test(f.from) || !/T.*(?:Z|[+-]\d\d:\d\d)$/.test(f.to) || from >= to || to - from > 366 * 86400000) throw bad('请选择不超过366天的有效起止时间，结束时间不包含在内。');
  for (const key of auditFields) if (typeof f[key] !== 'string' || f[key] && !(identifiers.has(key) ? id(f[key]) : /^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$/.test(f[key]))) throw bad('请填写准确的技术标识；人员与资源标识需为UUID。');
  return f;
}

export function auditWindow(from: string, to: string): AuditFilter {
  for (const date of [from, to]) { const parsed = new Date(date + 'T00:00:00Z'); if (!/^\d{4}-\d\d-\d\d$/.test(date) || !Number.isFinite(parsed.getTime()) || parsed.toISOString().slice(0, 10) !== date) throw bad('日期无效。'); }
  return validateAuditFilter({ from: from + 'T00:00:00+08:00', to: to + 'T00:00:00+08:00', actor_id: '', executor_id: '', channel: '', action: '', result: '', resource_type: '', resource_id: '', request_id: '', run_id: '', job_id: '' });
}

export function currentAuditFilter(now = new Date()): AuditFilter {
  const hkt = new Date(now.getTime() + 8 * 3600000), end = new Date(hkt);
  end.setUTCDate(end.getUTCDate() + 1);
  return auditWindow(hkt.toISOString().slice(0, 7) + '-01', end.toISOString().slice(0, 10));
}

export function auditKey(f: AuditFilter): string {
  validateAuditFilter(f);
  return JSON.stringify([f.from, f.to, ...auditFields.map(key => f[key])]);
}

export function auditFilterSummary(f: AuditFilter): string {
  validateAuditFilter(f);
  const date = (value: string) => new Date(Date.parse(value) + 8 * 3600000).toISOString().slice(0, 16).replace('T', ' ');
  return [date(f.from) + ' → ' + date(f.to) + ' 前（香港）', ...auditFields.filter(key => f[key]).map(key => auditLabels[key] + '：' + f[key])].join(' · ');
}

export function auditPath(f: AuditFilter, cursor = '', limit = 20): string {
  validateAuditFilter(f);
  if (!Number.isInteger(limit) || limit < 1 || limit > 100 || cursor.length > 2048) throw bad();
  const values: Record<string, string> = { from: f.from, to: f.to, ...Object.fromEntries(auditFields.filter(key => f[key]).map(key => [key, f[key]])), limit: String(limit), ...(cursor ? { cursor } : {}) };
  return '/audit/events?' + Object.entries(values).map(([key, value]) => key + '=' + encodeURIComponent(value)).join('&');
}

export function checkedAuditEvent(e: AuditEvent, eventID?: string): AuditEvent {
  if (!e || !id(e.id) || eventID && e.id !== eventID || !Number.isFinite(Date.parse(e.occurred_at))) throw bad();
  for (const field of ['actor_id', 'executor_id', 'resource_id', 'run_id', 'job_id'] as const) if (e[field] !== null && !id(e[field])) throw bad();
  for (const field of ['channel', 'action', 'result', 'resource_type', 'request_id'] as const) if (typeof e[field] !== 'string' || e[field].length > 4096) throw bad();
  if (e.resource_version !== null && (typeof e.resource_version !== 'string' || !/^-?[0-9]{1,20}$/.test(e.resource_version))) throw bad();
  for (const field of ['before_digest', 'after_digest'] as const) if (e[field] !== null && typeof e[field] !== 'string') throw bad();
  if ('reason' in e || 'session_id' in e || 'payload' in e) throw bad('通用审计响应混入了内容字段，已停止显示。');
  return e;
}

export function checkedAuditPage(p: AuditPage, f: AuditFilter): AuditPage {
  if (!p || p.disclosure !== 'organization_metadata_v1' || auditKey(p.filters) !== auditKey(f) || !Array.isArray(p.items) || p.items.length > 100 || p.next_cursor !== null && (typeof p.next_cursor !== 'string' || !p.next_cursor || p.next_cursor.length > 2048)) throw bad();
  const seen = new Set<string>(); let previous: AuditEvent | undefined;
  for (const item of p.items) {
    checkedAuditEvent(item);
    const time = Date.parse(item.occurred_at);
    if (seen.has(item.id) || time < Date.parse(f.from) || time >= Date.parse(f.to) || previous && (time > Date.parse(previous.occurred_at) || item.occurred_at === previous.occurred_at && item.id >= previous.id)) throw bad();
    for (const field of auditFields) if (f[field] && (identifiers.has(field) ? item[field]?.toLowerCase() !== f[field].toLowerCase() : item[field] !== f[field])) throw bad();
    seen.add(item.id); previous = item;
  }
  return p;
}

export function checkedAuditExport(e: AuditExport, f?: AuditFilter, exportID?: string): AuditExport {
  if (!e || !id(e.id) || exportID && e.id !== exportID || e.disclosure !== 'organization_metadata_v1' || !Object.prototype.hasOwnProperty.call(auditExportStates, e.state) || e.can_download !== (e.state === 'ready') || !hash(e.csv_sha256) || !Number.isSafeInteger(e.row_count) || e.row_count < 0 || e.row_count > 10000 || !Number.isSafeInteger(e.csv_bytes) || e.csv_bytes < 1 || e.csv_bytes > AUDIT_BYTES) throw bad('固定导出回执无效，请核对我的导出记录。');
  if (!Number.isFinite(Date.parse(e.created_at)) || !Number.isFinite(Date.parse(e.expires_at)) || Date.parse(e.created_at) >= Date.parse(e.expires_at) || Date.parse(e.expires_at) - Date.parse(e.created_at) > 86401000) throw bad();
  validateAuditFilter(e.filters);
  if (f && auditKey(e.filters) !== auditKey(f)) throw bad('导出条件与确认范围不一致。');
  return e;
}

export function checkedAuditExportUpdate(current: AuditExport, previous: AuditExport): AuditExport {
  checkedAuditExport(current, previous.filters, previous.id);
  for (const key of ['created_at', 'expires_at', 'row_count', 'csv_bytes', 'csv_sha256'] as const) if (current[key] !== previous[key]) throw bad('固定导出内容发生变化，已停止下载。');
  if (!current.can_download) throw bad('导出已过期或权限变化，请重新生成。');
  return current;
}

export async function readAuditBytes(record: AuditExport, fetchChunk: (offset: number, length: number) => Promise<AuditChunk>, sha256: (bytes: Uint8Array) => Promise<string> | string, active: () => void): Promise<Uint8Array> {
  checkedAuditExport(record);
  if (!record.can_download) throw bad('导出已过期或权限变化，请重新生成。');
  const out = new Uint8Array(record.csv_bytes);
  try {
    for (let offset = 0; offset < out.length;) {
      active(); const length = Math.min(AUDIT_CHUNK, out.length - offset), part = await fetchChunk(offset, length); active();
      if (part.export_id !== record.id || part.filename !== 'audit-' + record.id + '.csv' || part.sha256 !== record.csv_sha256 || part.total_size_bytes !== String(out.length) || part.offset !== String(offset) || part.length !== String(length) || !hash(part.chunk_sha256)) throw bad('下载分段与固定导出不一致。');
      const data = decodeBase64(part.data_base64);
      try { if (data.length !== length || await sha256(data) !== part.chunk_sha256) throw bad('下载分段校验失败。'); active(); out.set(data, offset); offset += length; } finally { data.fill(0); }
    }
    active(); if (await sha256(out) !== record.csv_sha256) throw bad('完整文件摘要不一致。'); active(); return out;
  } catch (error) { out.fill(0); throw error; }
}
