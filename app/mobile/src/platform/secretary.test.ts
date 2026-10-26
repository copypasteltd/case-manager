import test from 'node:test';
import assert from 'node:assert/strict';
import { messageText, runName, runReceipt } from './secretary';
import { MemoryDrafts } from './drafts';
import type { Schemas } from './session';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const expected = { conversation: id(1), actor: id(2), profile: id(3), message: id(4) };
const run = { id: id(5), conversation_id: expected.conversation, actor_id: expected.actor, profile_id: expected.profile, client_message_id: expected.message, version: 1, last_event_seq: 1, state: 'queued', can_cancel: true };

test('a run receipt must belong to the exact actor conversation profile and client message', () => {
  assert.equal(runReceipt(run, expected), true);
  for (const field of ['conversation_id', 'actor_id', 'profile_id', 'client_message_id']) assert.equal(runReceipt({ ...run, [field]: id(10) }, expected), false, field);
  for (const value of [null, {}, { ...run, id: '-'.repeat(36) }, { ...run, version: 0 }, { ...run, version: 1.5 }, { ...run, last_event_seq: 2 }, { ...run, state: 'unknown' }, { ...run, can_cancel: 'yes' }]) assert.equal(runReceipt(value, expected), false);
});

test('a reconciled terminal receipt remains valid without inventing a new queued task', () => {
  assert.equal(runReceipt({ ...run, version: 3, last_event_seq: 3, state: 'cancelled', can_cancel: false }, expected), true);
});

test('a successful secretary turn distinguishes a response from completion of its business request',()=>{
  const value={...run,state:'succeeded',can_cancel:false} as Schemas['Run'];
  assert.equal(runName({...value,outcome:'awaiting_input'}),'已回复，等待补充资料');
  assert.equal(runName({...value,outcome:'failed'}),'已回复，事项未完成');
  assert.equal(runName({...value,outcome:'completed'}),'已回复，本次事项已完成');
  assert.equal(runName(value),'已回复，请核对事项结果');
});

test('unavailable source messages never render a supplied stale body', () => {
  assert.equal(messageText({ visibility: 'available', body: '<script>literal</script>' } as Schemas['Message']), '<script>literal</script>');
  assert.equal(messageText({ visibility: 'unavailable', body: 'previous confidential text' } as Schemas['Message']).includes('previous confidential text'), false);
});

test('secretary drafts keep input text and request identity while excluding source contents and permissions', () => {
  const drafts = new MemoryDrafts(() => ({ authenticated: true, epoch: 1 }));
  const authority = { epoch: 1, userID: expected.actor, authorizationVersion: 1 };
  const target = { kind: 'run-create' as const, id: expected.conversation, version: 1 };
  const handle = drafts.open(authority, target)!;
  assert.equal(handle.write({ reason: '', action: expected.profile, owner: expected.message, body: '尚未确认的请求', source_refs: [{ id: id(9), body: 'secret' }], approved: true, mfa: '123456' } as never), true);
  const saved = handle.read()!;
  assert.equal(saved.body, '尚未确认的请求'); assert.equal(saved.owner, expected.message);
  assert.equal('source_refs' in saved, false); assert.equal('approved' in saved, false); assert.equal('mfa' in saved, false);
  assert.equal(drafts.open(authority, { ...target, version: 2 })!.read(), undefined);
  drafts.open({ ...authority, authorizationVersion: 2 }, target);
  assert.equal(handle.read(), undefined);
});
