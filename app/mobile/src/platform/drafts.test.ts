import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MemoryDrafts, type DraftAuthority, type DraftTarget } from './drafts';
import { currentProjectInput } from './project-input';
import { currentResourceInput } from './project-resource-input';
import type { Schemas } from './session';

function harness() {
  let now = 1000000; const session = { authenticated: true, epoch: 1 };
  const drafts = new MemoryDrafts(() => session, () => now);
  const authority: DraftAuthority = { epoch: 1, userID: 'owned-user', authorizationVersion: 1 };
  const target: DraftTarget = { kind: 'task', id: 'owned-task', version: 1 };
  return { drafts, session, authority, target, advance(ms: number) { now += ms; }, open(value: DraftTarget = target, auth: DraftAuthority = authority) { return drafts.open(auth, value)!; } };
}
const fields = { reason: 'Unsubmitted human reason', action: 'start' };

test('resource inputs share authority and item versions, with defensive copies and no retained command evidence', () => {
  const h = harness(), target: DraftTarget = { kind: 'resource-edit', id: 'risk:owned-risk', version: 4 };
  const input = { ...currentResourceInput({ owner_id: 'owner' } as Schemas['Project']), title: 'Actual risk', mitigation: 'Plan before leaving', confirmed: true, issue_id: 'not-retained', body: { access_token: 'not-retained' } };
  const handle = h.open(target); assert.equal(handle.write({ ...fields, resource: input }), true);
  input.mitigation = 'Caller changed'; const read = handle.read()!; read.resource!.mitigation = 'Reader changed';
  assert.equal(h.open(target).read()?.resource?.mitigation, 'Plan before leaving');
  assert.deepEqual(Object.keys(handle.read()!.resource!).sort(), ['title', 'owner', 'due', 'description', 'severity', 'probability', 'mitigation', 'resolution'].sort());
  assert.equal(h.open({ ...target, version: 5 }).read(), undefined);
  h.open(target, { ...h.authority, authorizationVersion: 2 }); assert.equal(handle.read(), undefined);
});

test('resource text competes for the same total memory budget and expires without reviving stale decisions', () => {
  const h = harness(), handles = [];
  for (let i = 0; i < 4; i++) {
    const handle = h.open({ kind: 'resource-create', id: 'issue:project-' + i, version: 1 }); handles.push(handle);
    assert.equal(handle.write({ ...fields, resource: { ...currentResourceInput({ owner_id: 'owner' } as Schemas['Project']), description: 'd'.repeat(20000), resolution: 'r'.repeat(20000) } }), true);
  }
  assert.equal(handles[0].read(), undefined); assert.equal(handles[3].read()?.resource?.description.length, 20000);
  assert.equal(handles[3].write({ ...fields, resource: { ...handles[3].read()!.resource!, description: 'x'.repeat(20001) } }), false);
  h.advance(15 * 60000); assert.equal(handles[3].read(), undefined);
  h.session.authenticated = false; assert.equal(handles[3].write(fields), false);
});

test('project text restores only for its exact authority and version and never retains template evidence', () => {
  const h = harness(), target: DraftTarget = { kind: 'project-edit', id: 'actual-project', version: 7 };
  const project = { ...currentProjectInput(), name: 'Human project', scope: 'Input before leaving', template_id: 'not retained', confirmed: true };
  const handle = h.open(target);
  assert.equal(handle.write({ ...fields, project }), true);
  project.scope = 'Caller mutation';
  assert.deepEqual(handle.read()?.project, { ...currentProjectInput(), name: 'Human project', scope: 'Input before leaving' });
  const restored = handle.read()!; restored.project!.scope = 'Reader mutation';
  assert.equal(h.open(target).read()?.project?.scope, 'Input before leaving');
  assert.equal(h.open({ ...target, version: 8 }).read(), undefined);
  h.open(target, { ...h.authority, authorizationVersion: 2 });
  assert.equal(handle.read(), undefined);
});
test('project text participates in the shared memory budget and logout immediately invalidates retained input', () => {
  const h = harness(), handles = [];
  for (let i = 0; i < 6; i++) {
    const handle = h.open({ kind: 'project-create', id: 'project-' + i, version: 1 }); handles.push(handle);
    assert.equal(handle.write({ ...fields, project: { ...currentProjectInput(), scope: 'x'.repeat(20000) } }), true);
  }
  assert.equal(handles[0].read(), undefined);
  assert.equal(handles[5].read()?.project?.scope.length, 20000);
  h.session.authenticated = false;
  assert.equal(handles[5].read(), undefined);
});

test('form input survives page recreation within the same authority and resource version, without shared mutable objects', () => {
  const h = harness(), copy = { ...fields }; h.open().write(copy); copy.reason = 'mutated caller';
  const restored = h.open().read()!; assert.deepEqual(restored, fields); restored.reason = 'mutated reader'; assert.deepEqual(h.open().read(), fields);
  h.open().forget(); assert.equal(h.open().read(), undefined);
});
test('logout, a new login and explicit clearing invalidate old handles before they can restore or resurrect input', () => {
  const h = harness(), old = h.open(); old.write(fields); h.session.authenticated = false;
  assert.equal(old.read(), undefined); assert.equal(old.write(fields), false);
  h.drafts.clear(); h.session.authenticated = true; h.session.epoch = 2;
  const fresh = h.open(h.target, { ...h.authority, epoch: 2, userID: 'another-user' });
  assert.equal(fresh.read(), undefined); assert.equal(old.write(fields), false); fresh.write(fields); h.drafts.clear(); assert.equal(fresh.read(), undefined);
});
test('a newer server authorization version clears drafts and an older page cannot roll the authority backward', () => {
  const h = harness(), old = h.open(); old.write(fields);
  const fresh = h.open(h.target, { ...h.authority, authorizationVersion: 2 });
  assert.equal(fresh.read(), undefined); assert.equal(old.write(fields), false);
  fresh.write({ ...fields, reason: 'Current permitted input' }); assert.equal(h.drafts.open(h.authority, h.target), undefined); assert.equal(fresh.read()?.reason, 'Current permitted input');
  assert.equal(h.drafts.open({ ...h.authority, userID: 'different-user' }, h.target), undefined);
});
test('task versions and every fixed approval revision or step remain separate and do not inherit an old decision', () => {
  const h = harness(); h.open().write(fields); assert.equal(h.open({ ...h.target, version: 2 }).read(), undefined);
  const target: DraftTarget = { kind: 'approval', id: 'request', version: 2, revision: 'revision-a', hash: 'hash-a', step: 'step-a', stepVersion: 1 };
  h.open(target).write({ reason: 'Review fixed terms', action: 'reject' });
  for (const changed of [{ version: 3 }, { revision: 'revision-b' }, { hash: 'hash-b' }, { step: 'step-b' }, { stepVersion: 2 }]) assert.equal(h.open({ ...target, ...changed }).read(), undefined);
  assert.equal(h.open(target).read()?.action, 'reject');
});
test('reading a draft does not prolong its 15-minute input lifetime', () => {
  const h = harness(), handle = h.open(); handle.write(fields); h.advance(14 * 60000); assert.deepEqual(handle.read(), fields);
  h.advance(60000); assert.equal(handle.read(), undefined); handle.write(fields); h.advance(60000); assert.deepEqual(handle.read(), fields);
});
test('bounded drafts evict oldest input, cap total text and refuse oversized fields without truncating a human reason', () => {
  const h = harness(); for (let i = 0; i < 33; i++) h.open({ ...h.target, id: 'task-' + i }).write(fields);
  assert.equal(h.open({ ...h.target, id: 'task-0' }).read(), undefined); assert.deepEqual(h.open({ ...h.target, id: 'task-32' }).read(), fields);
  h.drafts.clear(); for (let i = 0; i < 6; i++) h.open({ ...h.target, id: 'large-' + i }).write({ reason: 'x'.repeat(20000), action: 'reject' });
  assert.equal(h.open({ ...h.target, id: 'large-0' }).read(), undefined); assert.equal(h.open({ ...h.target, id: 'large-5' }).read()?.reason.length, 20000);
  const handle = h.open(); handle.write(fields); assert.equal(handle.write({ ...fields, reason: 'x'.repeat(20001) }), false); assert.deepEqual(handle.read(), fields);
});

test('deliverable input stays bound to the exact form, head and fixed acceptance revision', () => {
  const h = harness();
  const edit: DraftTarget = { kind: 'deliverable-edit', id: 'delivery', version: 4 };
  const saved = { reason: 'Clarify acceptance', action: 'save', name: 'Final source', criteria: 'Verify the original files', owner: 'owner-id', milestone: 'milestone-id' };
  assert.equal(h.open(edit).write(saved), true);
  assert.deepEqual(h.open(edit).read(), saved);
  for (const kind of ['deliverable-create', 'deliverable-submit', 'comment-create', 'comment-edit'] as const) assert.equal(h.open({ ...edit, kind }).read(), undefined);
  assert.equal(h.open({ ...edit, version: 5 }).read(), undefined);
  const decision: DraftTarget = { kind: 'deliverable-decision', id: 'delivery', version: 5, revision: 'fixed-a', hash: 'a'.repeat(64) };
  h.open(decision).write({ reason: 'Actually reviewed this revision', action: 'accept' });
  assert.equal(h.open({ ...decision, revision: 'fixed-b' }).read(), undefined);
  assert.equal(h.open({ ...decision, hash: 'b'.repeat(64) }).read(), undefined);
  assert.equal(h.drafts.open(h.authority, { ...decision, hash: 'invalid' }), undefined);
  assert.equal(h.open(decision).read()?.action, 'accept');
});

test('only whitelisted textual input is retained; file selections, credentials and human verification never restore', () => {
  const h = harness(), handle = h.open({ kind: 'deliverable-submit', id: 'delivery', version: 1 });
  const input = { reason: 'Submit source', action: 'submit', description: 'Exact deliverable', files_verified: true, criteria_verified: true, fresh: true, code: '123456', access_token: 'never-copy', file_version_ids: ['file-version'], payload: { secret: 'never-copy' } };
  assert.equal(handle.write(input), true);
  assert.deepEqual(handle.read(), { reason: 'Submit source', action: 'submit', description: 'Exact deliverable' });
  const read = handle.read()!; read.description = 'Changed by caller';
  assert.equal(handle.read()?.description, 'Exact deliverable');
  const fresh = h.open({ kind: 'deliverable-submit', id: 'delivery', version: 1 }, { ...h.authority, authorizationVersion: 2 });
  assert.equal(fresh.read(), undefined); assert.equal(handle.write(input), false);
});

test('new text fields share the memory budget and reject overlong content without replacing valid input', () => {
  const h = harness(), target: DraftTarget = { kind: 'deliverable-edit', id: 'delivery', version: 1 };
  const input = { ...fields, name: 'A', criteria: 'x'.repeat(20000), description: 'y'.repeat(20000) };
  const first = h.open(target); assert.equal(first.write(input), true);
  assert.equal(first.write({ ...input, name: 'n'.repeat(201) }), false);
  assert.deepEqual(first.read(), input);
  h.open({ ...target, id: 'second' }).write(input); h.open({ ...target, id: 'third' }).write(input);
  assert.equal(first.read(), undefined); assert.equal(h.open({ ...target, id: 'third' }).read()?.criteria?.length, 20000);
});
