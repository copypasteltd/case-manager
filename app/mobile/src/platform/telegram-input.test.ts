import test from 'node:test';
import assert from 'node:assert/strict';
import { bindingReceipt, challengeReceipt, channelInput, channelPayload, channelReceipt, endedReceipt, telegramReason,
  type Binding, type Challenge, type Channel } from './telegram-input';

const id = (n: number) => '00000000-0000-4000-8000-' + String(n).padStart(12, '0');
const at = '2026-10-26T12:00:00Z';
const bot = { bot_id: '9999999999999999', username: 'CopyPasteTestBot' };
function input() { return { ...channelInput(), bot: bot.bot_id, chat: '-9999999999999999', label: ' synthetic channel ' }; }
function body() { return channelPayload(input(), undefined, ' explicit reason ', true, true); }
function channel(): Channel { return { id: id(1), version: 1, created_at: at, updated_at: at, bot_id: bot.bot_id,
  chat_id: input().chat, topic_id: 0, label: 'synthetic channel', project_id: null, allowed_skill_ids: ['project-briefing'],
  default_skill_id: null, state: 'paused', disclosure_mode: 'portal_only', delivery_enabled: false }; }
function challenge(): Challenge { const token = 'a'.repeat(43); return { id: id(2), version: 1, bot_id: bot.bot_id,
  telegram_user_id: null, expires_at: at, created_at: at, claimed_at: null, confirmed_at: null, cancelled_at: null,
  start_token: token, start_url: 'https://t.me/' + bot.username + '?start=' + token }; }
function binding(): Binding { return { id: id(3), version: 1, bot_id: bot.bot_id, telegram_user_id: '9999999999999998', state: 'active', created_at: at, revoked_at: null }; }

test('TG identifiers keep all sixteen decimal digits and explicit zero topic without number coercion', () => {
  const value = body(); assert.equal('bot_id' in value && value.bot_id, bot.bot_id); assert.equal('chat_id' in value && value.chat_id, '-9999999999999999');
  assert.equal('topic_id' in value && value.topic_id, 0); assert.equal(value.reason, 'explicit reason'); assert.equal(value.label, 'synthetic channel');
  assert.equal(value.project_id, null); assert.equal(value.delivery_enabled, false);
});
test('TG topic rejects empty, rounded, exponent, signed and noncanonical integers', () => {
  for (const topic of ['', '1.2', '+1', '01', '1e3', '-1', 'Infinity', '2147483648', '9007199254740993']) assert.throws(() => channelPayload({ ...input(), topic }, undefined, 'reason', true, true));
  assert.equal('topic_id' in channelPayload({ ...input(), topic: '2147483647' }, undefined, 'reason', true, true), true);
});
test('TG bot and group identity reject unsafe or display-name aliases', () => {
  for (const chat of ['group', '100', '-01', '-0', '-10000000000000000', '-1.1']) assert.throws(() => channelPayload({ ...input(), chat }, undefined, 'reason', true, true));
  for (const bot of ['@someone', '0', '01', '10000000000000000']) assert.throws(() => channelPayload({ ...input(), bot }, undefined, 'reason', true, true));
});
test('TG channel requires explicit current MFA and complete scope acknowledgement', () => {
  assert.throws(() => channelPayload(input(), undefined, 'reason', false, true));
  assert.throws(() => channelPayload(input(), undefined, 'reason', true, false));
  assert.throws(() => channelPayload({ ...input(), project: 'unselected-title' }, undefined, 'reason', true, true));
});
test('TG defaults must remain in a unique nonempty allowed skill set', () => {
  assert.throws(() => channelPayload({ ...input(), allowed: [] }, undefined, 'reason', true, true));
  assert.throws(() => channelPayload({ ...input(), allowed: ['project-briefing', 'project-briefing'] }, undefined, 'reason', true, true));
  assert.throws(() => channelPayload({ ...input(), defaultSkill: 'incident-summary' }, undefined, 'reason', true, true));
  assert.throws(() => channelPayload({ ...input(), allowed: ['write-anything'] as never }, undefined, 'reason', true, true));
});
test('TG new channel is paused and cannot silently enable ingress', () => {
  assert.throws(() => channelPayload({ ...input(), state: 'active' }, undefined, 'reason', true, true));
  assert.equal('state' in body(), false); assert.equal(channelReceipt(channel(), body()), true);
  assert.equal(channelReceipt({ ...channel(), state: 'active' }, body()), false);
});
test('TG patch binds old revision and refuses replacing an existing exact location', () => {
  const current = channel(), value = channelPayload({ ...channelInput(current), state: 'active' }, current, 'reviewed scope', true, true);
  assert.equal('expected_version' in value && value.expected_version, 1); assert.equal('bot_id' in value, false);
  for (const delta of [{ bot: '2' }, { chat: '-2' }, { topic: '2' }]) assert.throws(() => channelPayload({ ...channelInput(current), ...delta }, current, 'reason', true, true));
});
test('TG receipt detects substituted identity, scope, permissions, revision and delivery policy', () => {
  const expected = body(); assert.equal(channelReceipt(channel(), expected), true);
  for (const delta of [{ bot_id: '2' }, { chat_id: '-2' }, { topic_id: 2 }, { version: 2 }, { project_id: id(8) },
    { allowed_skill_ids: ['incident-summary'] }, { default_skill_id: 'project-briefing' }, { delivery_enabled: true }, { disclosure_mode: 'group_body' }, { updated_at: 'invalid' }]) {
    assert.equal(channelReceipt({ ...channel(), ...delta }, expected), false);
  }
  assert.equal(channelReceipt({}, expected), false); assert.equal(channelReceipt(null, expected), false);
});
test('TG actual patch receipt must advance exactly one revision on the same channel', () => {
  const current = channel(), draft = { ...channelInput(current), state: 'active' as const, allowed: ['incident-summary', 'project-briefing'] as Channel['allowed_skill_ids'] };
  const expected = channelPayload(draft, current, 'reason', true, true), saved = { ...channel(), version: 2, state: 'active', allowed_skill_ids: ['project-briefing', 'incident-summary'] };
  assert.equal(channelReceipt(saved, expected, current), true);
  for (const delta of [{ id: id(77) }, { version: 1 }, { version: 3 }]) assert.equal(channelReceipt({ ...saved, ...delta }, expected, current), false);
});
test('TG private capability accepts only the exact selected Bot URL and returned token', () => {
  assert.equal(challengeReceipt(challenge(), bot), true);
  for (const delta of [{ start_url: 'https://t.me.evil.invalid/' }, { start_url: challenge().start_url + '&other=1' },
    { start_token: 'b'.repeat(43) }, { start_token: undefined }, { bot_id: '2' }, { telegram_user_id: '3' }, { expires_at: 'bad' }]) assert.equal(challengeReceipt({ ...challenge(), ...delta }, bot), false);
});
test('TG binding receipt corroborates the proved numeric identity rather than a display name', () => {
  const proved = { ...challenge(), telegram_user_id: binding().telegram_user_id };
  assert.equal(bindingReceipt(binding(), proved), true);
  for (const delta of [{ telegram_user_id: '3' }, { bot_id: '3' }, { state: 'revoked' }, { id: 'arbitrary' }, { revoked_at: at }]) assert.equal(bindingReceipt({ ...binding(), ...delta }, proved), false);
  assert.equal(bindingReceipt(binding(), challenge()), false);
});
test('TG cancellation and revocation receipt bind the exact object, identity and next revision', () => {
  const b = binding(), c = challenge();
  assert.equal(endedReceipt({ ...b, version: 2, state: 'revoked', revoked_at: at }, b, 'binding'), true);
  assert.equal(endedReceipt({ ...c, version: 2, cancelled_at: at }, c, 'challenge'), true);
  for (const delta of [{ id: id(99) }, { version: 1 }, { bot_id: '4' }, { telegram_user_id: '5' }, { revoked_at: null }]) assert.equal(endedReceipt({ ...b, version: 2, state: 'revoked', revoked_at: at, ...delta }, b, 'binding'), false);
});
test('TG copies keep a separate selected skill list and normalize only explicit human text', () => {
  const current = channel(), value = channelInput(current); value.allowed.push('incident-summary'); assert.deepEqual(current.allowed_skill_ids, ['project-briefing']);
  assert.equal(telegramReason('  确认本次范围  '), '确认本次范围'); assert.throws(() => telegramReason('  ')); assert.throws(() => telegramReason('字'.repeat(201)));
});
