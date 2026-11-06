import test from 'node:test';
import assert from 'node:assert/strict';
import type { components } from '../../../packages/contracts/src/api';
import { fixedDeliverySourceRunID, assertFixedDeliveryRun, fixedDeliveryNavigation, matchingNotificationDelivery } from '../../../packages/contracts/src/fixed-delivery-navigation';

type S = components['schemas'];
const id = (n: number) => '00000000-0000-4000-8000-' + String(n).padStart(12, '0');
const hash = 'a'.repeat(64);
const source = { type: 'agent_fixed_delivery', id: id(1), resource_version: 1, version_id: null, sha256: hash } as S['AgentSourceSnapshot'];
const scopeRun = { id: id(1), conversation_id: id(2), fixed_delivery: { run_id: id(1), kind: 'scope_transfer', input: { plan_id: id(3), expected_version: 1, plan_hash: hash, reviewed: true, authorized: true, reason: 'reviewed' }, preview: null, intent_sha256: hash, operation_id: id(4), result: { state: 'executed', old_authority_invalidated: true } } } as unknown as S['Run'];

test('fixed delivery sources select their exact run rather than an unsupported resource detail', () => {
  assert.equal(fixedDeliverySourceRunID(source), id(1));
  assert.equal(fixedDeliverySourceRunID({ ...source, version_id: undefined }), id(1));
  assert.equal(fixedDeliverySourceRunID({ ...source, type: 'file' }), undefined);
  assert.equal(fixedDeliverySourceRunID({ ...source, id: 'not-an-id' }), undefined);
  assert.equal(assertFixedDeliveryRun(scopeRun, id(1), id(2), source).id, id(1));
  assert.throws(() => assertFixedDeliveryRun(scopeRun, id(1), id(9), source));
  assert.throws(() => assertFixedDeliveryRun(scopeRun, id(1), id(2), { ...source, sha256: 'b'.repeat(64) }));
});

test('post-transfer source navigation retains receipt identity and no old file route', () => {
  const result = fixedDeliveryNavigation(scopeRun);
  assert.equal(result?.kind, 'scope_transfer');
  if (result?.kind !== 'scope_transfer') throw new Error('missing receipt');
  assert.equal(result.planID, id(3));
  assert.equal(result.operationID, id(4));
  assert.equal(result.executed, true);
  assert.equal(result.source, undefined);
  assert.equal(result.target, undefined);
});

test('the actual notification ledger is correlated by request, source run and delivery identity', () => {
  const notification = { id: id(1), conversation_id: id(2), fixed_delivery: { run_id: id(1), kind: 'notification', input: { request_id: id(3), expected_version: 5, body_sha256: hash, start_char: 0, max_chars: 4, reviewed: true, disclosed_to_group: true }, preview: { request_id: id(3), run_id: id(6), run_version: 5, body_sha256: hash, total_chars: 10 }, intent_sha256: hash, operation_id: id(4), result: { id: id(7), delivery_id: id(8), run_id: id(6), body_sha256: hash, start_char: 0, max_chars: 4 } } } as unknown as S['Run'];
  const receipt = fixedDeliveryNavigation(notification);
  if (receipt?.kind !== 'notification') throw new Error('missing receipt');
  const delivery = { id: id(8), request_id: id(3), run_id: id(6), phase: 'reviewed_result', state: 'delivery_unknown' } as S['TelegramDelivery'];
  assert.equal(matchingNotificationDelivery({ deliveries: [delivery], events: [] }, receipt).delivery.state, 'delivery_unknown');
  assert.throws(() => matchingNotificationDelivery({ deliveries: [{ ...delivery, request_id: id(9) }], events: [] }, receipt));
  assert.throws(() => matchingNotificationDelivery({ deliveries: [], events: [] }, receipt));
});
