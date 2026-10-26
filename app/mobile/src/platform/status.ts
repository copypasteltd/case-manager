import type { Schemas } from './session';

export type Service = Schemas['Service'];
export type StatusSnapshot = { value: Service; startedAt: number; expiresAt: number };
export const statusNames: Record<Service['state'], string> = { operational: '运行正常', degraded: '性能下降', down: '服务异常', maintenance: '维护中', unknown: '状态未知' };
export const statusTones: Record<Service['state'], string> = { operational: 'success', degraded: 'warning', down: 'danger', maintenance: 'info', unknown: 'neutral' };
export const incidentNames = { open: '待处理', investigating: '调查中', monitoring: '恢复观察中', resolved: '已解决' } as const;
export const severityNames = { low: '低', medium: '中', high: '高', critical: '紧急' } as const;
export const observationReasons: Record<string, string> = { LATE_SEQUENCE: '序号迟到', LATE_OBSERVATION: '观测时间迟到', STALE_SAMPLE: '到达时已过期' };
export const sourceReasons: Record<string, string> = { SOURCE_DISABLED: '来源已停用', NO_OBSERVATION: '尚无有效观测', OBSERVATION_STALE: '观测已过期', RECEIPT_STALE: '接收已过期', CLOCK_SKEW: '时钟异常', INVALID_STATE: '状态待核对' };

// Capture before sending, not after receiving: all network time consumes the
// server's lease. Never compare a device wall clock with server timestamps.
export function stampStatus(value: Service, startedAt: number): StatusSnapshot {
  const lifetime = Date.parse(value.recheck_at) - Date.parse(value.evaluated_at);
  const valid = Number.isFinite(startedAt) && startedAt >= 0 && Number.isFinite(lifetime) && lifetime > 0;
  return { value, startedAt, expiresAt: valid ? startedAt + Math.min(lifetime, 10000) : 0 };
}
export function statusIsCurrent(snapshot: StatusSnapshot, now: number) {
  return Number.isFinite(now) && now >= snapshot.startedAt && snapshot.expiresAt > now;
}
// Clock resets, exceptions and unavailable high-resolution clocks fail closed.
// A fresh page lifetime can acquire a new clock; Date.now is never a fallback.
export function monotonicReader(read: () => number): () => number {
  let last = -1, failed = false;
  return () => {
    if (failed) return NaN;
    try { const now = read(); if (!Number.isFinite(now) || now < 0 || now < last) failed = true; else last = now; }
    catch { failed = true; }
    return failed ? NaN : last;
  };
}
export function statusTime(input: string | null | undefined) {
  if (!input) return '未提供';
  const value = new Date(input);
  if (!Number.isFinite(value.getTime())) return '时间无效';
  return value.toLocaleString('zh-CN', { timeZone: 'Asia/Hong_Kong', hour12: false }) + '（香港 UTC+8）';
}
