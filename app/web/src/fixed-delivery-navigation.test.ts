import { describe, expect, it } from 'vitest';
import type { components } from '@copypaste/contracts';
import { fixedDeliveryNavigation, fixedDeliverySourceRunID, assertFixedDeliveryRun, organizationPlanSourceMatches, organizationOutcomeTarget, matchingNotificationDelivery } from '@copypaste/contracts/fixed-delivery-navigation';

type S = components['schemas'];
const id = (n: number) => '00000000-0000-4000-8000-' + String(n).padStart(12, '0');
const hash = 'a'.repeat(64), conversation = id(2);
function run(kind: 'scope_transfer' | 'notification', executed = false): S['Run'] {
  const input = kind === 'scope_transfer'
    ? { plan_id: id(3), expected_version: 2, plan_hash: hash, reviewed: true, authorized: true, reason: 'reviewed' }
    : { request_id: id(3), expected_version: 4, body_sha256: hash, start_char: 2, max_chars: 5, reviewed: true, disclosed_to_group: true };
  const preview = kind === 'scope_transfer'
    ? executed ? null : { id: id(3), version: 2, plan_hash: hash, input: { kind: 'file', resource_id: id(7), target_folder_id: id(8) }, snapshot: { source: { kind: 'file', id: id(7) }, target: { kind: 'folder', id: id(8) } } }
    : { request_id: id(3), run_id: id(9), run_version: 4, body_sha256: hash, total_chars: 20 };
  const result = !executed ? null : kind === 'scope_transfer'
    ? { state: 'executed', old_authority_invalidated: true }
    : { id: id(10), delivery_id: id(11), run_id: id(9), body_sha256: hash, start_char: 2, max_chars: 5 };
  return { id: id(1), conversation_id: conversation, fixed_delivery: { run_id: id(1), kind, input, preview, intent_sha256: hash, operation_id: executed ? id(12) : null, result } } as unknown as S['Run'];
}
const source = { type: 'agent_fixed_delivery', id: id(1), resource_version: 1, version_id: null, sha256: hash } as S['AgentSourceSnapshot'];

describe('fixed delivery result navigation', () => {
  it('uses the source run ID and rejects a different intent or conversation', () => {
    expect(fixedDeliverySourceRunID(source)).toBe(id(1));
    expect(fixedDeliverySourceRunID({ ...source, version_id: undefined })).toBe(id(1));
    expect(fixedDeliverySourceRunID({ ...source, type: 'file' })).toBeUndefined();
    expect(assertFixedDeliveryRun(run('scope_transfer', true), id(1), conversation, source).id).toBe(id(1));
    expect(() => assertFixedDeliveryRun(run('scope_transfer', true), id(1), id(99), source)).toThrow();
    expect(() => assertFixedDeliveryRun(run('scope_transfer', true), id(1), conversation, { ...source, sha256: 'b'.repeat(64) })).toThrow();
  });
  it('keeps a committed scope receipt useful without reconstructing hidden old file access', () => {
    const pending = fixedDeliveryNavigation(run('scope_transfer'));
    expect(pending?.kind === 'scope_transfer' && pending.source).toEqual({ kind: 'file', id: id(7) });
    const receipt = fixedDeliveryNavigation(run('scope_transfer', true));
    expect(receipt).toMatchObject({ kind: 'scope_transfer', planID: id(3), operationID: id(12), executed: true, source: undefined, target: undefined });
    const leaked = run('scope_transfer', true);
    leaked.fixed_delivery!.preview = run('scope_transfer').fixed_delivery!.preview;
    expect(() => fixedDeliveryNavigation(leaked)).toThrow();
  });
  it('binds notification receipts to the original completed source run, not the execution run', () => {
    const value = fixedDeliveryNavigation(run('notification', true));
    expect(value).toMatchObject({ kind: 'notification', sourceRunID: id(9), requestID: id(3), deliveryID: id(11), reviewID: id(10) });
    const wrong = run('notification', true);
    wrong.fixed_delivery!.result!.run_id = wrong.id;
    expect(() => fixedDeliveryNavigation(wrong)).toThrow();
    const changed = run('notification', true);
    changed.fixed_delivery!.result!.max_chars = 6;
    expect(() => fixedDeliveryNavigation(changed)).toThrow();
  });
  it('shows only the exact current authorized notification delivery and its own events', () => {
    const receipt = fixedDeliveryNavigation(run('notification', true));
    if (receipt?.kind !== 'notification') throw new Error('missing receipt');
    const delivery = { id: id(11), request_id: id(3), run_id: id(9), phase: 'reviewed_result', state: 'sent', version: 2 } as S['TelegramDelivery'];
    const event = { id: id(13), delivery_id: id(11), version: 2, state: 'sent' } as S['TelegramDeliveryEvent'];
    expect(matchingNotificationDelivery({ deliveries: [delivery], events: [event, { ...event, id: id(14), delivery_id: id(15) }] }, receipt)).toEqual({ delivery, events: [event] });
    expect(() => matchingNotificationDelivery({ deliveries: [{ ...delivery, run_id: id(1) }], events: [] }, receipt)).toThrow();
    expect(() => matchingNotificationDelivery({ deliveries: [delivery, delivery], events: [] }, receipt)).toThrow();
  });
  it('opens only the exact organization plan and ordinary result identity', () => {
    const plan = { id: id(3), conversation_id: conversation, actor_id: id(4), plan_hash: hash, state: 'committed', preview: [{ action: { kind: 'file.move', id: id(7), target_folder_id: id(8) }, source: { id: id(7) }, target: { id: id(8) } }], results: [{ operation_id: id(12), data: { id: id(7), folder_id: id(8) } }] } as unknown as S['AgentOrganizationPlan'];
    const ref = { ...source, type: 'agent_organization_plan', id: plan.id } as S['AgentSourceSnapshot'];
    expect(organizationPlanSourceMatches(plan, ref, conversation, id(4))).toBe(true);
    expect(organizationPlanSourceMatches(plan, ref, id(99), id(4))).toBe(false);
    expect(organizationOutcomeTarget(plan, 0)).toEqual({ kind: 'file', id: id(7) });
    plan.results[0].data.id = id(99);
    expect(organizationOutcomeTarget(plan, 0)).toBeUndefined();
  });
});
