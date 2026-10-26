import test from 'node:test';
import assert from 'node:assert/strict';
import { CommandKeys, UnresolvedCommand } from './command-keys';

test('a page recreated while a write is in flight reuses the same exact intent key', () => {
  let created = 0; const keys = new CommandKeys(() => 'intent-' + (++created));
  const key = keys.reserve('user/auth/project/create', 'exact original body');
  assert.equal(keys.reserve('user/auth/project/create', 'exact original body'), key);
  assert.throws(() => keys.reserve('user/auth/project/create', 'changed body'), UnresolvedCommand);
  assert.equal(created, 1);
  keys.settle('user/auth/project/create', key, 'succeeded', false);
  assert.equal(keys.reserve('user/auth/project/create', 'exact original body'), key);
  keys.settle('user/auth/project/create', key, 'succeeded', true);
  const next = keys.reserve('user/auth/project/create', 'exact original body');
  assert.notEqual(next, key);
  keys.settle('user/auth/project/create', key, 'succeeded', false);
  assert.equal(keys.reserve('user/auth/project/create', 'exact original body'), next, 'A late hidden page cannot resurrect or consume a newer operation');
});

test('uncertain outcomes retain the original key and forbid silently substituting another request', () => {
  let created = 0; const keys = new CommandKeys(() => 'intent-' + (++created));
  const key = keys.reserve('delivery/head/revision/hash', 'fixed file IDs and decision');
  keys.settle('delivery/head/revision/hash', key, 'uncertain');
  assert.equal(keys.reserve('delivery/head/revision/hash', 'fixed file IDs and decision'), key);
  assert.throws(() => keys.reserve('delivery/head/revision/hash', 'other file IDs'), UnresolvedCommand);
  keys.settle('delivery/head/revision/hash', key, 'rejected');
  assert.notEqual(keys.reserve('delivery/head/revision/hash', 'corrected input'), key);
  assert.notEqual(keys.reserve('delivery/other-head/revision/hash', 'corrected input'), key);
});

test('bounded command records refuse new work without evicting unresolved keys; logout clears the old lifetime', () => {
  let created = 0; const keys = new CommandKeys(() => 'intent-' + (++created), 30, 2);
  const first = keys.reserve('first', '1234567890'); keys.reserve('second', '123456789');
  assert.throws(() => keys.reserve('third', 'x'), UnresolvedCommand);
  assert.equal(keys.reserve('first', '1234567890'), first);
  keys.clear(); assert.notEqual(keys.reserve('first', '1234567890'), first);
  const limited = new CommandKeys(() => 'key', 10);
  assert.throws(() => limited.reserve('scope', '123456'), UnresolvedCommand);
});
