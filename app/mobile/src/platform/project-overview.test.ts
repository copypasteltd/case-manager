import test from 'node:test';
import assert from 'node:assert/strict';
import type { Schemas } from './session';
import { currentOverview, exactAmount, financialKind, financialReadPath, financialURL, overviewClock, overviewCollectionURL, overviewItemURL, overviewLabels, overviewState, type OverviewKind } from './project-overview';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
function snapshot(): Schemas['ProjectOverview'] {
  return { project_id: id(1), project_version: 7, as_of: '2026-10-25T16:30:09Z', scope: 'readable_records',
    groups: (Object.keys(overviewLabels) as OverviewKind[]).map(kind => ({ kind, available: false, count: null, states: [], items: [], has_more: false })),
    incurred: { available: false, totals: [], unconfirmed_count: null }, budgets: [] };
}
test('financial decimal text preserves huge, negative, zero and six-place source facts without rounding', () => {
  assert.equal(exactAmount('45035996273704.950000'), '45,035,996,273,704.950000');
  assert.equal(exactAmount('-90071992547409.900001'), '-90,071,992,547,409.900001');
  assert.equal(exactAmount('0.000000'), '0.000000');
  assert.equal(exactAmount('1000'), '1,000');
  assert.equal(exactAmount('0.01'), '0.01');
});
test('missing, scientific, non-finite and non-contract money never display an invented zero', () => {
  for (const value of [null, undefined, '', '1e6', 'NaN', 'Infinity', '1.1234567', 'HKD100', '12,000']) assert.equal(exactAmount(value), '金额未核实');
});
test('snapshot clocks retain Hong Kong midnight and seconds independently of device timezone', () => {
  assert.equal(overviewClock('2026-10-25T16:30:09Z'), '2026-10-26 00:30:09 · 香港');
  assert.equal(overviewClock('2026-10-26T00:30:09+08:00'), '2026-10-26 00:30:09 · 香港');
  assert.equal(overviewClock('invalid'), '时间未核实');
  assert.equal(overviewState('active'), '使用中', 'Service configuration is not a health claim');
});
test('all sixteen associated kinds have actual native routes and keep their stable identity', () => {
  for (const kind of Object.keys(overviewLabels) as OverviewKind[]) {
    const url = overviewItemURL(id(1), kind, id(2));
    assert.match(url, /^\/pages\/(?:project|finance|detail|business|deliverable|status)\/index\?/);
    assert.ok(url.includes(id(2)), kind + ' loses the selected record');
  }
  assert.equal(overviewItemURL(id(1), 'budgets', id(2)), '/pages/project/index?id=' + id(1) + '&section=budgets&focus=' + id(2));
  assert.equal(overviewItemURL(id(1), 'posts', id(2)), '/pages/detail/index?kind=post&id=' + id(2));
  assert.equal(overviewItemURL(id(1), 'services', id(2)), '/pages/status/index?kind=service&id=' + id(2));
});
test('collection links preserve actual project scope and never advertise global services as project-filtered', () => {
  assert.equal(overviewCollectionURL(id(1), 'services'), null);
  assert.equal(overviewCollectionURL(id(1), 'tasks'), '/pages/list/index?kind=tasks&project_id=' + id(1));
  assert.equal(overviewCollectionURL(id(1), 'changes'), '/pages/project/index?id=' + id(1) + '&section=changes');
  assert.equal(overviewCollectionURL(id(1), 'subscriptions'), '/pages/list/index?kind=subscriptions&project_id=' + id(1));
});
test('financial reads use a fixed allowlist and canonical UUIDs instead of arbitrary route/tool input', () => {
  assert.equal(financialReadPath('cost', id(2).toUpperCase()), '/costs/' + id(2));
  assert.equal(financialURL('subscription', id(2)), '/pages/finance/index?kind=subscription&id=' + id(2));
  for (const value of ['__proto__', 'constructor', '/users', 'https://outside.example', null]) assert.throws(() => financialKind(value));
  assert.throws(() => financialReadPath('payment', '../../outside'));
});
test('an accurate authorized overview preserves unknown categories as unknown rather than readable zero', () => {
  const value = snapshot();
  assert.equal(currentOverview(value, id(1), 7), value);
  assert.equal(value.groups[0].count, null);
  const readable = value.groups[0]; readable.available = true; readable.count = 0;
  assert.equal(currentOverview(value, id(1), 7).groups[0].count, 0);
});
test('foreign, stale project versions, incomplete categories and invalid snapshot clocks fail closed', () => {
  assert.throws(() => currentOverview(snapshot(), id(2), 7));
  assert.throws(() => currentOverview(snapshot(), id(1), 8));
  const missing = snapshot(); missing.groups.pop(); assert.throws(() => currentOverview(missing, id(1), 7));
  const duplicate = snapshot(); duplicate.groups[1] = duplicate.groups[0]; assert.throws(() => currentOverview(duplicate, id(1), 7));
  const clock = snapshot(); clock.as_of = 'invalid'; assert.throws(() => currentOverview(clock, id(1), 7));
});
test('unavailable counts, titles or state statistics cannot enter the factual view', () => {
  for (const patch of [{ count: 0 }, { states: [{ state: 'todo', count: 1 }] }, { has_more: true }, { items: [{ id: id(2), version: 1, revision_id: null, title: 'Hidden title', state: 'todo', updated_at: '2026-10-26T00:00:00Z' }] }]) {
    const value = snapshot(); Object.assign(value.groups[0], patch);
    assert.throws(() => currentOverview(value, id(1), 7));
  }
});
test('full readable count is independent of five recent previews; inconsistent states or pagination fail closed', () => {
  const value = snapshot(), group = value.groups[0];
  Object.assign(group, { available: true, count: 8, states: [{ state: 'todo', count: 8 }], has_more: true,
    items: Array.from({ length: 5 }, (_, n) => ({ id: id(n + 10), version: 1, revision_id: null, title: 'Task ' + n, state: 'todo', updated_at: '2026-10-26T00:00:00Z' })) });
  assert.equal(currentOverview(value, id(1), 7).groups[0].count, 8);
  group.has_more = false; assert.throws(() => currentOverview(value, id(1), 7)); group.has_more = true;
  group.states[0].count = 7; assert.throws(() => currentOverview(value, id(1), 7)); group.states[0].count = 8;
  group.items[1].id = group.items[0].id; assert.throws(() => currentOverview(value, id(1), 7));
});
