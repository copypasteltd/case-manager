import test from 'node:test';
import assert from 'node:assert/strict';
import type { Schemas } from './session';
import { changeBasisKey, changeBudgetProof, changeDate, changePayload, changeReceipt, changeRequestProof, changeWriter,
  copyChangeInput, currentChange, currentChangeBasis, currentChangeInput, currentChangePayload, currentChangeRevision,
  implementPayload, implementationProof, savedChangeProof, sameChangeBasis } from './change-input';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const project = { id: id(1), version: 7, name: 'Actual project', project_no: 'CP-1', owner_id: id(2), creator_id: id(2), type: 'internal',
  state: 'active', scope: 'Original scope', classification: 'project_restricted', start_date: '2028-02-29', due_date: '2028-12-31' } as Schemas['Project'];
const me = { user: { id: id(2) }, actions: ['change.create', 'change.update', 'change.submit', 'change.implement', 'project.update', 'budget.read'] } as Schemas['Me'];
const members = { items: [], memberships: [{ user_id: me.user.id, project_role: 'manager', authorization_version: 1 }] } as Schemas['ProjectMembers'];
const file = { file_id: id(3), version_id: id(4), seq: 2, name: 'Actual fixed evidence.txt', size_bytes: '123456789123456789', sha256: 'a'.repeat(64), media_type: 'text/plain', scan_state: 'clean', scan_waiver_ref: null } as Schemas['BusinessFileReference'];
const line = { category: 'Delivery', period_start: '2028-02-29', period_end: '2028-12-31', amount: '100.000000' };
function basis(): Schemas['ChangeBasis'] { return { project_id: project.id, project_version: project.version, name: project.name, scope: project.scope,
  start_date: project.start_date!, due_date: project.due_date!, classification: project.classification!, currency: 'HKD', budget_id: null, budget_lines: [], budget_total: '0.000000' }; }
function input() { return { ...currentChangeInput(basis()), title: ' Actual proposal ', category: ' Delivery ', scope: ' New complete scope ', scopeDelta: ' Explicit scope impact ', scheduleDelta: ' Explicit schedule impact ' }; }
function draft(b = basis(), before?: Schemas['Change']): Schemas['Change'] {
  const data = changePayload({ ...input(), budgetMode: 'replace', lines: [{ id: 'line-1', category: line.category, start: line.period_start, end: line.period_end, amount: '100' }] }, b, project, [file], ' Actual reason ', true, before);
  const rid = id(before ? 21 : 20), changeID = id(10);
  const revision: Schemas['ChangeRevision'] = { id: rid, change_id: changeID, seq: before ? before.revision.seq + 1 : 1,
    base_revision_id: before?.current_revision_id ?? null, payload_schema_version: 1,
    content: { ...data.content, budget_lines: [line] }, basis: b, content_hash: 'b'.repeat(64), approval_amount: '100.000000', budget_delta: '100.000000',
    target_budget_id: id(before ? 23 : 22), author_id: me.user.id, reason: data.reason, created_at: '2028-02-29T01:00:00Z', attachments: [file] };
  return { id: changeID, version: before ? before.version + 1 : 1, project_id: project.id, creator_id: me.user.id, classification: project.classification!,
    created_at: '2028-02-29T01:00:00Z', updated_at: '2028-02-29T01:00:00Z', state: 'draft', lifecycle: 'draft', approval_state: 'not_submitted',
    current_revision_id: rid, approved_revision_id: null, request_id: null, revision, can_edit: true, can_prepare_request: true, can_implement: false, implementation: null };
}
function request(change: Schemas['Change'], state = 'approved' as Schemas['Request']['state']) { return { id: id(30), version: 3, kind: 'change', business_id: change.id,
  state, current_revision_id: id(31), content_hash: 'c'.repeat(64), payload: currentChangePayload(change), can_submit: false, can_revise: false, can_withdraw: false } as Schemas['Request']; }
function approved(b = basis()) { const c = draft(b); return { ...c, state: 'approved', lifecycle: 'requested', approval_state: 'approved', approved_revision_id: c.current_revision_id, request_id: id(30), can_edit: false, can_prepare_request: false, can_implement: true } as Schemas['Change']; }
function implemented(old = approved()) {
  const q = request(old), body = implementPayload(old, project, old.revision.basis, q, ' Checked original approval ', true, true, true);
  const actualProject = { ...project, version: project.version + 1, scope: old.revision.content.scope, start_date: old.revision.content.start_date ?? undefined, due_date: old.revision.content.due_date ?? undefined };
  const budget: Schemas['ProjectBudget'] = { id: old.revision.target_budget_id!, project_id: project.id, revision: 1, state: 'approved', currency: 'HKD', creator_id: me.user.id,
    change_id: old.id, change_revision_id: old.current_revision_id, predecessor_id: old.revision.basis.budget_id, lines: [line], total: '100.000000', created_at: '2028-02-29T01:01:00Z' };
  const actualBasis = { ...old.revision.basis, project_version: actualProject.version, scope: actualProject.scope, start_date: actualProject.start_date!, due_date: actualProject.due_date!, budget_id: budget.id, budget_lines: budget.lines, budget_total: budget.total };
  const receipt = { id: old.id, revision_id: old.current_revision_id, version: old.version + 1 };
  const head: Schemas['Change'] = { ...old, version: receipt.version, state: 'implemented', lifecycle: 'effective', can_implement: false, implementation: {
    id: id(40), change_id: old.id, revision_id: old.current_revision_id, request_revision_id: q.current_revision_id, project_id: project.id, actor_id: me.user.id,
    before_project_version: project.version, after_project_version: actualProject.version, change_version: receipt.version, scope_verified: true, budget_verified: true,
    reason: body.reason, mfa_verified_at: '2028-02-29T01:00:30Z', created_at: '2028-02-29T01:01:00Z' } };
  return { old, body, receipt, head, actualProject, actualBasis, budget };
}
const clone = <T>(value: T): T => structuredClone(value);

test('accepted basis binds full project version, scope, dates, classification, currency and all budget lines', () => {
  const b = basis(); assert.equal(currentChangeBasis(b, project, 'HKD'), b);
  for (const patch of [{ project_version: 8 }, { project_id: id(99) }, { scope: 'Other scope' }, { name: 'Other project' }, { currency: 'USD' }, { start_date: null }, { classification: 'internal' }]) assert.throws(() => currentChangeBasis({ ...b, ...patch }, project, 'HKD'));
  assert.throws(() => currentChangeBasis({ ...b, budget_lines: [line] }, project, 'HKD'));
});
test('basis fingerprints are stable across object property order and change for budget identity or details', () => {
  const b = basis(), reversed = Object.fromEntries(Object.entries(b).reverse()) as typeof b;
  assert.equal(changeBasisKey(b), changeBasisKey(reversed)); assert.equal(sameChangeBasis(b, reversed), true);
  assert.notEqual(changeBasisKey(b), changeBasisKey({ ...b, budget_id: id(50), budget_total: '100.000000', budget_lines: [line] }));
});
test('dates enforce real Gregorian days, inclusive ordered periods and explicit null dates', () => {
  assert.equal(changeDate('2028-02-29'), true);
  for (const value of ['2027-02-29', '2028-04-31', '0000-01-01', '2028-2-01', '2028-01-01T00:00:00Z']) assert.equal(changeDate(value), false);
  const p = changePayload({ ...input(), start: '', due: '' }, basis(), project, [], 'Clear dates', true);
  assert.equal(p.content.start_date, null); assert.equal(p.content.due_date, null);
  assert.throws(() => changePayload({ ...input(), start: '2028-12-31', due: '2028-02-29' }, basis(), project, [], 'Wrong dates', true));
});
test('keep null, full replacement and explicit empty replacement remain distinct without floating point arithmetic', () => {
  const i = { ...input(), lines: [{ id: 'one', category: ' Category ', start: '2028-02-29', end: '2028-12-31', amount: '99999999999999.000001' }] };
  assert.equal(changePayload(i, basis(), project, [], 'Keep', true).content.budget_lines, null);
  const replacement = changePayload({ ...i, budgetMode: 'replace' }, basis(), project, [], 'Replace', true);
  assert.equal(replacement.content.budget_lines![0].amount, '99999999999999.000001'); assert.equal(replacement.content.budget_lines![0].category, 'Category');
  assert.deepEqual(changePayload({ ...i, budgetMode: 'replace', lines: [] }, basis(), project, [], 'Clear', true).content.budget_lines, []);
});
test('overlap, invalid decimal formats, excess precision and row limits fail before a command is sent', () => {
  const row = { id: 'one', category: 'Delivery', start: '2028-02-29', end: '2028-06-30', amount: '100.00' };
  for (const value of ['-1', '1e3', '1,000', '01', '100000000000000', '1.1234567', 'NaN']) assert.throws(() => changePayload({ ...input(), budgetMode: 'replace', lines: [{ ...row, amount: value }] }, basis(), project, [], 'Check', true));
  assert.throws(() => changePayload({ ...input(), budgetMode: 'replace', lines: [row, { ...row, id: 'two', start: '2028-06-30', end: '2028-12-31' }] }, basis(), project, [], 'Overlap', true));
  assert.doesNotThrow(() => changePayload({ ...input(), budgetMode: 'replace', lines: [row, { ...row, id: 'two', start: '2028-07-01', end: '2028-12-31' }] }, basis(), project, [], 'Adjacent', true));
  assert.equal(copyChangeInput({ ...input(), lines: Array.from({ length: 101 }, (_, i) => ({ ...row, id: String(i) })) }), undefined);
});
test('saved inputs whitelist independent editable text and stable row identities, excluding evidence and authority', () => {
  const i = { ...input(), lines: [{ id: 'stable-row', category: 'Delivery', start: '', end: '', amount: '' }], files: [file], basis: basis(), fresh: true, scope_verified: true, access_token: 'never-copy' };
  const copy = copyChangeInput(i)!; assert.deepEqual(Object.keys(copy).sort(), [...Object.keys(input()).filter(k => k !== 'lines'), 'lines'].sort());
  copy.lines[0].category = 'Changed by caller'; assert.equal(i.lines[0].category, 'Delivery');
  assert.equal(copyChangeInput({ ...i, lines: [i.lines[0], i.lines[0]] }), undefined);
  assert.equal(copyChangeInput({ ...i, budgetMode: 'infer' }), undefined);
});
test('writer affordances honor actual member role, operation authority and read-only project phases', () => {
  assert.equal(changeWriter(project, members, me, 'change.create'), true);
  for (const state of ['closing', 'archived', 'cancelled'] as const) assert.equal(changeWriter({ ...project, state }, members, me, 'change.create'), false);
  assert.equal(changeWriter(project, { ...members, memberships: [{ ...members.memberships![0], project_role: 'viewer' }] }, me, 'change.update'), false);
  assert.equal(changeWriter(project, members, { ...me, actions: ['change.create'] }, 'change.create'), false);
});
test('new proposals require fresh explicit basis acceptance, complete text and ordered unique actual file versions', () => {
  const b = basis(), i = input(), second = { ...file, version_id: id(5) };
  assert.throws(() => changePayload(i, b, project, [file], 'Check', false));
  assert.throws(() => changePayload({ ...i, scopeDelta: '\u0085 ' }, b, project, [file], 'Check', true));
  assert.throws(() => changePayload(i, b, project, [file, file], 'Check', true));
  assert.deepEqual(changePayload(i, b, project, [second, file], ' Ordered ', true).content.file_version_ids, [second.version_id, file.version_id]);
});
test('editing appends an exact revision and freezes currency, prior head version and source revision', () => {
  const before = draft(), p = changePayload(input(), basis(), project, [file], 'Edit', true, before) as Schemas['ChangePatch'];
  assert.equal(p.expected_version, before.version); assert.equal(p.base_revision_id, before.current_revision_id);
  assert.throws(() => changePayload({ ...input(), currency: 'USD' }, { ...basis(), currency: 'USD' }, project, [file], 'Other currency', true, before));
  assert.throws(() => changePayload(input(), basis(), project, [file], 'Locked', true, { ...before, can_edit: false }));
  assert.equal(currentChangeInput({ ...basis(), start_date: '2029-01-01' }, { ...before.revision, content: { ...before.revision.content, start_date: null } }).start, '');
});
test('complete current revisions preserve source attachments and reject mismatched, duplicate, malformed or incomplete proof', () => {
  const c = draft(); assert.equal(currentChange(c, c.id), c);
  for (const mutate of [(v: any) => v.revision.id = id(99), (v: any) => v.revision.basis.project_id = id(99),
    (v: any) => v.revision.attachments[0].version_id = id(99), (v: any) => delete v.can_implement,
    (v: any) => v.revision.content.file_version_ids.push(file.version_id), (v: any) => v.revision.content_hash = 'bad',
    (v: any) => v.revision.target_budget_id = null, (v: any) => v.revision.attachments[0].scan_state = 'pending']) {
    const bad = clone(c); mutate(bad); assert.throws(() => currentChange(bad, c.id));
  }
  assert.throws(() => currentChangeRevision(c.revision, id(99)));
});
test('a three-field save receipt never suffices without exact actual head, content, files, actor and frozen basis', () => {
  const c = draft(), i = { ...input(), budgetMode: 'replace', lines: [{ id: 'one', category: line.category, start: line.period_start, end: line.period_end, amount: '100' }] };
  const p = changePayload(i, basis(), project, [file], ' Actual reason ', true), receipt = { id: c.id, revision_id: c.current_revision_id, version: 1 };
  assert.equal(changeReceipt(receipt), true); assert.equal(savedChangeProof(c, receipt, p, basis(), [file], me.user.id), true);
  for (const mutate of [(v: any) => v.revision.author_id = id(99), (v: any) => v.revision.content.scope = 'Wrong applied scope',
    (v: any) => v.revision.basis.project_version++, (v: any) => v.revision.attachments[0].sha256 = 'f'.repeat(64), (v: any) => v.revision.reason = 'Other reason']) {
    const bad = clone(c); mutate(bad); assert.equal(savedChangeProof(bad, receipt, p, basis(), [file], me.user.id), false);
  }
});
test('request draft and revised request proof bind all original business payload fields and server amounts', () => {
  const c = draft(), q = request(c, 'draft'); assert.equal(changeRequestProof(q, c, 'draft'), true);
  for (const mutate of [(v: any) => v.payload.project_change_basis.project_version++, (v: any) => v.payload.amount = '0.000000',
    (v: any) => v.payload.project_change_budget_delta = '-1.000000', (v: any) => v.payload.attachments = [], (v: any) => v.payload.business_content_hash = 'd'.repeat(64)]) {
    const bad = clone(q); mutate(bad); assert.equal(changeRequestProof(bad, c, 'draft'), false);
  }
  assert.equal(changeRequestProof(q, c, 'draft', q), false);
  assert.equal(changeRequestProof({ ...q, version: q.version + 1, current_revision_id: id(32) }, c, 'draft', q), true);
});
test('implementation requires exact independent approval, current full basis, both human attestations and fresh MFA', () => {
  const c = approved(), q = request(c), b = basis();
  const p = implementPayload(c, project, b, q, ' Checked ', true, true, true);
  assert.equal(p.reason, 'Checked'); assert.equal(p.request_revision_id, q.current_revision_id);
  for (const [scope, budget, fresh] of [[false, true, true], [true, false, true], [true, true, false]]) assert.throws(() => implementPayload(c, project, b, q, 'Check', scope, budget, fresh));
  assert.throws(() => implementPayload(c, project, b, request(c, 'in_review'), 'Check', true, true, true));
  assert.throws(() => implementPayload({ ...c, approved_revision_id: id(99) }, project, b, q, 'Check', true, true, true));
  assert.throws(() => implementPayload(c, { ...project, version: 8 }, { ...b, project_version: 8 }, q, 'Stale basis', true, true, true));
});
test('implementation proof reconciles immutable head, actual project, budget, source approval and actor without trusting receipt alone', () => {
  const f = implemented();
  const check = (v = f) => implementationProof(v.receipt, v.head, v.actualProject, v.actualBasis, v.old, project, v.body, me.user.id, v.budget);
  assert.equal(check(), true);
  for (const mutate of [(v: any) => v.head.implementation.actor_id = id(99), (v: any) => v.head.implementation.request_revision_id = id(99),
    (v: any) => v.head.implementation.scope_verified = false, (v: any) => v.actualProject.version++, (v: any) => v.actualProject.scope = 'Wrong scope',
    (v: any) => v.actualBasis.budget_total = '101.000000', (v: any) => v.budget.state = 'draft', (v: any) => v.budget.change_revision_id = id(99)]) {
    const bad = clone(f); mutate(bad); try { assert.equal(check(bad), false); } catch (e) { if (e instanceof assert.AssertionError) throw e; }
  }
  assert.equal(implementationProof(f.receipt, f.head, f.actualProject, f.actualBasis, f.old, project, f.body, me.user.id), false);
});
test('replacing a previous budget requires actual superseded predecessor evidence; other currencies cannot satisfy proof', () => {
  const b = { ...basis(), budget_id: id(60), budget_total: '100.000000', budget_lines: [line] }, f = implemented(approved(b));
  const previous = { ...f.budget, id: b.budget_id, state: 'superseded', lines: b.budget_lines, total: b.budget_total } as Schemas['ProjectBudget'];
  assert.equal(changeBudgetProof(f.budget, f.old, 'approved'), true);
  assert.equal(changeBudgetProof({ ...f.budget, currency: 'USD' }, f.old, 'approved'), false);
  assert.equal(implementationProof(f.receipt, f.head, f.actualProject, f.actualBasis, f.old, project, f.body, me.user.id, f.budget), false);
  assert.equal(implementationProof(f.receipt, f.head, f.actualProject, f.actualBasis, f.old, project, f.body, me.user.id, f.budget, previous), true);
  assert.equal(implementationProof(f.receipt, f.head, f.actualProject, f.actualBasis, f.old, project, f.body, me.user.id, f.budget, { ...previous, state: 'approved' }), false);
});
test('server approval increase and signed net delta are retained independently in fixed snapshots', () => {
  const c = draft(); c.revision.approval_amount = '300.000000'; c.revision.budget_delta = '-200.000000';
  const payload = currentChangePayload(c); assert.equal(payload.amount, '300.000000'); assert.equal(payload.project_change_budget_delta, '-200.000000');
  assert.equal(typeof payload.content.budget_lines![0].amount, 'string'); assert.equal(payload.attachments[0].size_bytes, file.size_bytes);
});
