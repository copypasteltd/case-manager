import type { Schemas } from './api';

export type StatusSnapshot<T extends Schemas['Service'] = Schemas['Service']> = { value: T; expiresAt: number };

// Use a monotonic clock and charge the entire request duration against the
// server's evaluation window. A slow response cannot acquire a new lifetime
// just because it arrived late; a changed device wall clock cannot prolong it.
export function stampStatus<T extends Schemas['Service']>(value: T, requestStartedAt: number): StatusSnapshot<T> {
  const lifetime = Date.parse(value.recheck_at) - Date.parse(value.evaluated_at);
  const safe = Number.isFinite(lifetime) && Number.isFinite(requestStartedAt) && requestStartedAt >= 0 && lifetime > 0;
  return { value, expiresAt: safe ? requestStartedAt + Math.min(lifetime, 10_000) : 0 };
}

export function statusIsCurrent(snapshot: StatusSnapshot, now: number): boolean {
  return Number.isFinite(now) && now >= 0 && snapshot.expiresAt > now;
}

export const statusNames: Record<Schemas['Service']['state'], string> = {
  operational: '运行正常', degraded: '性能下降', down: '服务异常', maintenance: '维护中', unknown: '状态未知',
};
export const statusTone: Record<Schemas['Service']['state'], string> = {
  operational: 'success', degraded: 'warning', down: 'danger', maintenance: 'info', unknown: 'neutral',
};

export function statusTime(value?: string | null): string {
  if (!value) return '尚无记录';
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return '时间待核对';
  return new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date);
}
