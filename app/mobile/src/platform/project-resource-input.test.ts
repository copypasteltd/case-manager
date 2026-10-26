import test from 'node:test';
import assert from 'node:assert/strict';
import type { Schemas } from './session';
import { copyResourceInput, currentResourceInput, resourcePayload, resourceReceipt, resourceTime, resourceTimeInput, resourceTransition, resourceTransitionReceipt, resourceTransitions } from './project-resource-input';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const project = { id: id(1), version: 7, owner_id: id(2), state: 'active' } as Schemas['Project'];
const members = { items: [{ id: id(2), status: 'active' }, { id: id(3), status: 'disabled' }] } as Schemas['ProjectMembers'];
const input = { ...currentResourceInput(project), title: '\u0085 Current checkpoint \u3000', due: '2028-02-29 18:00:00.123456', description: 'Full actual description', probability: 'high', mitigation: 'A reviewed mitigation', resolution: 'Actual resolution' };
const milestone = { id: id(10), project_id: project.id, version: 4, title: 'Current checkpoint', owner_id: id(2), state: 'open', due_at: '2028-02-29T10:00:00.123456Z' } as Schemas['Milestone'];
const risk = { ...milestone, id: id(11), severity: 'medium', probability: 'high', description: input.description, mitigation: input.mitigation } as Schemas['Risk'];
const issue = { ...milestone, id: id(12), severity: 'medium', description: input.description, resolution: input.resolution } as Schemas['Issue'];

test('Hong Kong wall time is explicit, offset-equivalent and lossless at PostgreSQL microsecond precision', () => {
  assert.equal(resourceTime(input.due), '2028-02-29T10:00:00.123456Z');
  assert.equal(resourceTime('2028-02-29T05:00:00.123456-05:00'), resourceTime(input.due));
  assert.equal(resourceTime('2028-02-29 18:00'), '2028-02-29T10:00:00.000000Z');
  assert.equal(resourceTimeInput('2028-02-29T10:00:00.123456Z'), input.due);
  assert.equal(resourceTime(resourceTimeInput('2028-02-29T10:00:00.000001Z')), '2028-02-29T10:00:00.000001Z');
});
test('invalid calendar dates, fractional rounding and invalid time zones cannot silently normalize a decision', () => {
  for (const due of ['', '2026-02-29 12:00', '2028-02-30T12:00:00+08:00', '2028-01-01 24:00', '2028-01-01 12:60', '2028-01-01T12:00:60Z', '2028-01-01T12:00:00+24:00', '2028-01-01T12:00:00+08:60', '2028-01-01T12:00:00.1234567Z', '0000-01-01 12:00', '2028-01-01']) assert.throws(() => resourceTime(due), due);
});
test('all three creates use original paths and complete fields without invented parent expected-version or state', () => {
  const created = resourcePayload('milestone', input, project, members);
  assert.deepEqual(created, { path: '/projects/' + project.id + '/milestones', method: 'POST', body: { title: 'Current checkpoint', owner_id: id(2), due_at: resourceTime(input.due) } });
  const riskBody = resourcePayload('risk', input, project, members).body;
  assert.deepEqual(riskBody, { ...created.body, description: input.description, severity: 'medium', probability: 'high', mitigation: input.mitigation });
  assert.deepEqual(resourcePayload('issue', { ...input, due: '' }, project, members).body, { title: 'Current checkpoint', owner_id: id(2), description: input.description, severity: 'medium', resolution: input.resolution });
});
test('only actual active project members and writable project records can be selected or edited', () => {
  for (const owner of ['', id(3), id(90)]) assert.throws(() => resourcePayload('risk', { ...input, owner }, project, members));
  for (const state of ['closing', 'archived', 'cancelled'] as const) assert.throws(() => resourcePayload('milestone', input, { ...project, state }, members));
  assert.throws(() => resourcePayload('milestone', input, project, members, { ...milestone, project_id: id(99) }));
  for (const state of ['achieved', 'cancelled'] as const) assert.throws(() => resourcePayload('milestone', input, project, members, { ...milestone, state }));
  assert.throws(() => resourcePayload('risk', input, project, members, { ...risk, state: 'realized', issue_id: id(77) }));
  assert.throws(() => resourcePayload('issue', input, project, members, { ...issue, state: 'closed' }));
});
test('edits bind the item version and do not send unselected state changes or another entity-specific field', () => {
  for (const [kind, item] of [['milestone', milestone], ['risk', risk], ['issue', issue]] as const) {
    const command = resourcePayload(kind, input, project, members, item);
    assert.equal(command.method, 'PATCH'); assert.equal(command.body.expected_version, 4);
    for (const key of ['project_id', 'state', 'reason', 'issue_id', 'creator_id']) assert.equal(key in command.body, false);
    if (kind !== 'risk') assert.equal('probability' in command.body, false);
  }
});
test('optional dates and risk probability remain absent or exact, never falsely reported as cleared', () => {
  assert.equal('due_at' in resourcePayload('risk', { ...input, due: '', probability: '' }, project, members).body, false);
  assert.equal('probability' in resourcePayload('risk', { ...input, due: '', probability: '' }, project, members).body, false);
  assert.throws(() => resourcePayload('milestone', { ...input, due: '' }, project, members));
  assert.throws(() => resourcePayload('risk', { ...input, due: '' }, project, members, risk));
  assert.throws(() => resourcePayload('risk', { ...input, probability: '' }, project, members, risk));
});
test('successful creation and edit receipts prove actual identity, fields, project, state and exact version', () => {
  for (const [kind, item] of [['milestone', milestone], ['risk', risk], ['issue', issue]] as const) {
    const actual = { ...item, version: 1 };
    assert.equal(resourceReceipt(actual, kind, input, project), true);
    for (const patch of [{ id: 'invented' }, { project_id: id(99) }, { version: 2 }, { state: 'closed' }, { title: 'Other' }, { owner_id: id(3) }]) assert.equal(resourceReceipt({ ...actual, ...patch }, kind, input, project), false);
    assert.equal(resourceReceipt({ ...item, version: 5 }, kind, input, project, item), true);
    assert.equal(resourceReceipt({ ...item, id: id(98), version: 5 }, kind, input, project, item), false);
  }
});
test('receipt checks reject sub-millisecond differences and unexpected concern field or issue association changes', () => {
  assert.equal(resourceReceipt({ ...risk, version: 5, due_at: '2028-02-29T10:00:00.123457Z' }, 'risk', input, project, risk), false);
  assert.equal(resourceReceipt({ ...risk, version: 5, issue_id: id(88) }, 'risk', input, project, risk), false);
  assert.equal(resourceReceipt({ ...risk, version: 5, mitigation: 'Changed elsewhere' }, 'risk', input, project, risk), false);
  assert.equal(resourceReceipt({ ...issue, version: 5, resolution: 'Different resolution' }, 'issue', input, project, issue), false);
});
test('state choices respect the original graph and the server-derived overdue fact', () => {
  assert.deepEqual(resourceTransitions('milestone', milestone), ['achieve', 'cancel']);
  assert.deepEqual(resourceTransitions('milestone', { ...milestone, overdue: true }), ['achieve', 'miss', 'cancel']);
  assert.deepEqual(resourceTransitions('risk', { ...risk, state: 'realized' }), []);
  assert.deepEqual(resourceTransitions('issue', { ...issue, state: 'closed' }), ['open']);
  assert.throws(() => resourceTransition('risk', risk, project, 'open', 'Reason', '', ['issue.create']));
  assert.throws(() => resourceTransition('milestone', milestone, project, 'miss', 'Reason', '', []));
});
test('each transition fixes the exact version and reason; issue closure needs resolution and realization needs issue.create', () => {
  assert.deepEqual(resourceTransition('milestone', milestone, project, 'achieve', ' Reviewed evidence ', '', []), { path: '/milestones/' + milestone.id + '/transition', method: 'POST', body: { expected_version: 4, reason: 'Reviewed evidence', action: 'achieve' } });
  assert.throws(() => resourceTransition('risk', risk, project, 'realized', 'Actual occurrence', '', []));
  assert.equal(resourceTransition('risk', risk, project, 'realized', 'Actual occurrence', '', ['issue.create']).body.state, 'realized');
  assert.throws(() => resourceTransition('issue', issue, project, 'resolved', 'Investigated', ' ', []));
  assert.throws(() => resourceTransition('issue', issue, project, 'resolved', ' ', 'Actual result', []));
  assert.deepEqual(resourceTransition('issue', issue, project, 'resolved', 'Investigated', 'Full actual result', []).body, { expected_version: 4, reason: 'Investigated', state: 'resolved', resolution: 'Full actual result' });
});
test('transition receipts retain all unchanged facts and require a real new issue ID on risk realization', () => {
  assert.equal(resourceTransitionReceipt({ ...milestone, version: 5, state: 'achieved' }, 'milestone', milestone, 'achieve', ''), true);
  const realized = { ...risk, version: 5, state: 'realized', issue_id: id(50) };
  assert.equal(resourceTransitionReceipt(realized, 'risk', risk, 'realized', ''), true);
  for (const patch of [{ issue_id: undefined }, { issue_id: risk.id }, { mitigation: 'Forged' }, { version: 6 }]) assert.equal(resourceTransitionReceipt({ ...realized, ...patch }, 'risk', risk, 'realized', ''), false);
  assert.equal(resourceTransitionReceipt({ ...issue, version: 5, state: 'resolved', resolution: 'Actual new result' }, 'issue', issue, 'resolved', 'Actual new result'), true);
  assert.equal(resourceTransitionReceipt({ ...issue, version: 5, state: 'resolved' }, 'issue', issue, 'resolved', 'Actual new result'), false);
});
test('legacy template milestones without an owner or date can still cancel or reopen with exact receipts', () => {
  const old = { ...milestone, owner_id: null, due_at: null };
  assert.equal(resourceTransitionReceipt({ ...old, state: 'cancelled', version: 5 }, 'milestone', old, 'cancel', ''), true);
  const cancelled = { ...old, state: 'cancelled' as const };
  assert.equal(resourceTransitionReceipt({ ...cancelled, state: 'open', version: 5 }, 'milestone', cancelled, 'reopen', ''), true);
});
test('only bounded textual resource input is copied; API evidence and confirmation flags never become drafts', () => {
  const raw = { ...input, payload: { access_token: 'private' }, acknowledged: true, issue_id: id(50), expected_version: 999 };
  assert.deepEqual(copyResourceInput(raw), input);
  assert.equal(copyResourceInput({ ...input, mitigation: 'x'.repeat(20001) }), undefined);
  assert.equal(copyResourceInput({ ...input, owner: { id: id(2) } }), undefined);
  assert.equal(copyResourceInput(null), undefined);
});
