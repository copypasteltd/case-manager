import { describe, expect, it } from 'vitest';
import type { Schemas } from './api';
import { stampStatus, statusIsCurrent, statusTime } from './statusFreshness';

const service = (recheck = '2026-10-23T12:00:03Z') => ({ evaluated_at: '2026-10-23T12:00:00Z', recheck_at: recheck, state: 'operational' } as Schemas['Service']);

describe('status evidence lifetime', () => {
  it('expires at the maintenance or evidence boundary even before the next poll', () => {
    const view = stampStatus(service(), 1000);
    expect(statusIsCurrent(view, 3999)).toBe(true);
    expect(statusIsCurrent(view, 4000)).toBe(false);
  });
  it('charges slow responses against the deadline instead of renewing a green result', () => {
    const view = stampStatus(service(), 1000);
    expect(statusIsCurrent(view, 9000)).toBe(false);
    const later = stampStatus({ ...service(), evaluated_at: '2026-10-23T12:00:09Z', recheck_at: '2026-10-23T12:00:12Z' }, 9000);
    expect(statusIsCurrent(later, 9500)).toBe(true);
  });
  it('fails closed for absent or invalid deadlines and never accepts unbounded lifetimes', () => {
    for (const time of ['', 'invalid', '2026-10-23T12:00:00Z', '2026-10-23T11:59:59Z']) expect(statusIsCurrent(stampStatus(service(time), 1000), 1000)).toBe(false);
    expect(statusIsCurrent(stampStatus(service('2099-01-01T00:00:00Z'), 1000), 11000)).toBe(false);
    expect(statusIsCurrent(stampStatus(service(), -1), 1000)).toBe(false);
    expect(statusIsCurrent(stampStatus(service(), 1000), Number.NaN)).toBe(false);
  });
  it('formats the complete evidence instant in Hong Kong independently of device timezone', () => {
    expect(statusTime('2026-10-23T12:00:03Z')).toContain('20:00:03');
    expect(statusTime('invalid')).toBe('时间待核对');
    expect(statusTime(null)).toBe('尚无记录');
  });
});
