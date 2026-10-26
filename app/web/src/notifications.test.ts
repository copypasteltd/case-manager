import { describe, expect, it } from 'vitest';
import { mergeNotifications, notificationCount, notificationDestination, notificationReadInput, type Notification } from '@copypaste/contracts/notifications';
const id = 'c5ce4165-1d10-4f52-a2a5-d25df8674b62', parent = '9b20fc65-7065-4940-ad5f-39715703bad2';
const task = { type: 'task', id, container_id: parent, web_path: '/projects/' + parent + '?task=' + id } as const;
const notice = (delivery: number, read: number): Notification => ({ id, category: 'task', title: '任务待推进', resource_label: '当前任务', current_state: 'todo', attention_required: true, read: delivery === read, delivery_seq: delivery, read_seq: read, business_event_id: parent, template_version: 1, target: task, created_at: '2026-10-25T01:00:00Z', updated_at: '2026-10-25T01:00:00Z', read_at: null });
describe('business notification boundaries', () => {
  it('uses fixed routes for every supported business target', () => {
    expect(notificationDestination(task)).toEqual({ web: task.web_path, mobile: '/pages/detail/index?kind=task&id=' + id });
    for (const [type, prefix, kind] of [['request', '/requests/', 'detail'], ['contract', '/contracts/', 'business'], ['file', '/files/', 'detail'], ['incident', '/incidents/', 'status']] as const) expect(notificationDestination({ type, id, container_id: null, web_path: prefix + id }).mobile).toContain('/pages/' + kind + '/');
    expect(notificationDestination({ type: 'subscription_period', id, container_id: parent, web_path: '/subscription-periods/' + id }).mobile).toBe('/pages/subscription-period/index?id=' + id);
    expect(notificationDestination({ type: 'agent_run', id, container_id: parent, web_path: '/conversations/' + parent + '?run=' + id }).mobile).toContain('&run=' + id);
  });
  it('rejects external, mismatched, or malformed target addresses', () => {
    for (const web_path of ['https://example.org/', '//example.org/', task.web_path + '&redirect=evil', '/tasks/' + id, '/projects/' + id + '?task=' + parent]) expect(() => notificationDestination({ ...task, web_path })).toThrow();
    expect(() => notificationDestination({ ...task, container_id: null })).toThrow();
    expect(() => notificationDestination({ ...task, id: '../admin' })).toThrow();
    expect(() => notificationDestination({ ...task, type: 'unknown' } as never)).toThrow();
  });
  it('acknowledges only the delivery actually observed, independently of business duty', () => {
    expect(notificationReadInput(notice(2, 1))).toEqual({ delivery_seq: 2 });
    expect(notificationReadInput(notice(2, 2))).toEqual({ delivery_seq: 2 });
    for (const bad of [{ ...notice(2, 1), read: true }, notice(1, 2), notice(0, 0), notice(Number.MAX_SAFE_INTEGER + 1, 0)]) expect(() => notificationReadInput(bad)).toThrow();
  });
  it('retains newer deliveries and observed reads across overlapping pages', () => {
    const latest = notice(3, 1), stale = notice(2, 2);
    expect(mergeNotifications([latest, stale])).toEqual([latest]);
    expect(mergeNotifications([notice(3, 3), latest])).toEqual([notice(3, 3)]);
    expect(mergeNotifications([stale, latest])).toEqual([latest]);
  });
  it('does not present capped counts or incomplete zero as exact totals', () => {
    expect(notificationCount(100, false)).toBe('100+');
    expect(notificationCount(0, false)).toBe('待核对');
    expect(notificationCount(0, true)).toBe('0');
    expect(notificationCount(-1, true)).toBe('待核对');
  });
});
