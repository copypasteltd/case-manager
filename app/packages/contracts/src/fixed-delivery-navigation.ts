import type { components } from './api';

type S = components['schemas'];
type Source = S['AgentSourceSnapshot'];
export type OrdinaryFileTarget = { kind: 'file' | 'folder'; id: string };
type ReceiptBase = { runID: string; intentSHA256: string; operationID?: string };
export type ScopeDeliveryNavigation = ReceiptBase & {
  kind: 'scope_transfer'; planID: string; planHash: string; authorizedVersion: number;
  executed: boolean; source?: OrdinaryFileTarget; target?: OrdinaryFileTarget;
};
export type NotificationDeliveryNavigation = ReceiptBase & {
  kind: 'notification'; requestID: string; sourceRunID: string; authorizedVersion: number;
  bodySHA256: string; startChar: number; maxChars: number; reviewID?: string; deliveryID?: string;
};
export type FixedDeliveryNavigation = ScopeDeliveryNavigation | NotificationDeliveryNavigation;

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const sha256 = /^[0-9a-f]{64}$/;
function fail(): never { throw new Error('固定执行来源或普通业务回执尚未核对，请重新读取当前任务。'); }
function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail();
  return value as Record<string, unknown>;
}
function identifier(value: unknown): string { if (typeof value !== 'string' || !uuid.test(value)) return fail(); return value; }
function digest(value: unknown): string { if (typeof value !== 'string' || !sha256.test(value)) return fail(); return value; }
function integer(value: unknown, minimum = 1): number { if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < minimum) return fail(); return value; }
function target(value: unknown): OrdinaryFileTarget {
  const entry = object(value);
  if (entry.kind !== 'file' && entry.kind !== 'folder') return fail();
  return { kind: entry.kind, id: identifier(entry.id) };
}
function fixedSource(source: Source, type: Source['type']): boolean {
  return source.type === type && uuid.test(source.id) && sha256.test(source.sha256) && source.resource_version === 1 && source.version_id == null;
}

/** A fixed-delivery source ID is its execution run, not the enclosing message's run. */
export function fixedDeliverySourceRunID(source: Source): string | undefined {
  return fixedSource(source, 'agent_fixed_delivery') ? source.id : undefined;
}

export function assertFixedDeliveryRun(run: S['Run'], runID: string, conversationID: string, source?: Source): S['Run'] {
  if (run.id !== identifier(runID) || run.conversation_id !== identifier(conversationID)) return fail();
  if (source && (fixedDeliverySourceRunID(source) !== run.id || run.fixed_delivery?.run_id !== run.id || run.fixed_delivery.intent_sha256 !== source.sha256)) return fail();
  return run;
}

/** Reads only metadata already authorized by the run view. It never reloads an old scope plan. */
export function fixedDeliveryNavigation(run: S['Run']): FixedDeliveryNavigation | undefined {
  const view = run.fixed_delivery;
  if (!view) return undefined;
  if (identifier(view.run_id) !== identifier(run.id)) return fail();
  const base: ReceiptBase = { runID: run.id, intentSHA256: digest(view.intent_sha256) };
  if (view.operation_id !== null) base.operationID = identifier(view.operation_id);
  const input = object(view.input);
  if (input.reviewed !== true) return fail();
  const authorizedVersion = integer(input.expected_version);
  if (view.kind === 'scope_transfer') {
    if (input.authorized !== true) return fail();
    const planID = identifier(input.plan_id), planHash = digest(input.plan_hash);
    if (base.operationID) {
      const result = object(view.result);
      // The ordinary transfer invalidates the old authorization. A committed view
      // must retain only its minimal receipt, never a reconstructed source link.
      if (view.preview !== null || result.state !== 'executed' || result.old_authority_invalidated !== true) return fail();
      return { ...base, kind: 'scope_transfer', planID, planHash, authorizedVersion, executed: true, source: undefined, target: undefined };
    }
    if (view.result !== null) return fail();
    const preview = object(view.preview), snapshot = object(preview.snapshot), original = object(preview.input);
    if (preview.id !== planID || preview.plan_hash !== planHash || preview.version !== authorizedVersion) return fail();
    const source = target(snapshot.source), destination = target(snapshot.target);
    if (source.id !== original.resource_id || source.kind !== original.kind || destination.kind !== 'folder' || destination.id !== original.target_folder_id) return fail();
    return { ...base, kind: 'scope_transfer', planID, planHash, authorizedVersion, executed: false, source, target: destination };
  }
  if (view.kind !== 'notification' || input.disclosed_to_group !== true) return fail();
  const requestID = identifier(input.request_id), bodySHA256 = digest(input.body_sha256);
  const startChar = integer(input.start_char, 0), maxChars = integer(input.max_chars), preview = object(view.preview);
  if (maxChars > 2000 || preview.request_id !== requestID || preview.body_sha256 !== bodySHA256 || preview.run_version !== authorizedVersion || startChar + maxChars > integer(preview.total_chars, 0)) return fail();
  const sourceRunID = identifier(preview.run_id);
  const navigation: NotificationDeliveryNavigation = { ...base, kind: 'notification', requestID, sourceRunID, authorizedVersion, bodySHA256, startChar, maxChars };
  if (base.operationID) {
    const result = object(view.result);
    if (result.run_id !== sourceRunID || result.body_sha256 !== bodySHA256 || result.start_char !== startChar || result.max_chars !== maxChars) return fail();
    navigation.reviewID = identifier(result.id); navigation.deliveryID = identifier(result.delivery_id);
  } else if (view.result !== null) return fail();
  return navigation;
}

export function organizationPlanSourceMatches(plan: S['AgentOrganizationPlan'], source: Source, conversationID: string, actorID: string): boolean {
  return fixedSource(source, 'agent_organization_plan') && plan.id === source.id && plan.plan_hash === source.sha256 && plan.conversation_id === conversationID && plan.actor_id === actorID;
}

/** Ordinary result links are derived from a correlated per-item command receipt. */
export function organizationOutcomeTarget(plan: S['AgentOrganizationPlan'], index: number): OrdinaryFileTarget | undefined {
  if (plan.state !== 'committed' || plan.results.length !== plan.preview.length || index < 0 || !Number.isSafeInteger(index)) return undefined;
  const preview = plan.preview[index], outcome = plan.results[index];
  if (!preview || !outcome || !uuid.test(outcome.operation_id) || !uuid.test(outcome.data.id)) return undefined;
  const data = outcome.data as unknown as Record<string, unknown>, action = preview.action;
  if (action.kind === 'folder.create') return data.parent_id === action.id && preview.source.id === action.id ? { kind: 'folder', id: outcome.data.id } : undefined;
  if (outcome.data.id !== action.id || preview.source.id !== action.id) return undefined;
  if (action.target_folder_id ? preview.target?.id !== action.target_folder_id : preview.target !== null) return undefined;
  const parentID = action.target_folder_id ?? preview.source.parent_id;
  if (action.kind === 'folder.move' && data.parent_id === parentID) return { kind: 'folder', id: outcome.data.id };
  if (action.kind === 'file.move' && data.folder_id === parentID) return { kind: 'file', id: outcome.data.id };
  return undefined;
}

export function matchingNotificationDelivery(details: S['TelegramDeliveryDetails'], receipt: NotificationDeliveryNavigation): { delivery: S['TelegramDelivery']; events: S['TelegramDeliveryEvent'][] } {
  if (!receipt.deliveryID) return fail();
  const matches = details.deliveries.filter(value => value.id === receipt.deliveryID);
  if (matches.length !== 1) return fail();
  const delivery = matches[0];
  if (delivery.request_id !== receipt.requestID || delivery.run_id !== receipt.sourceRunID || delivery.phase !== 'reviewed_result') return fail();
  return { delivery, events: details.events.filter(value => value.delivery_id === delivery.id).sort((a, b) => a.version - b.version) };
}

export const fixedDeliveryStateNames: Record<S['TelegramDelivery']['state'], string> = {
  pending: '等待发送', sending: '正在发送', retry_wait: '等待重试', sent: 'Bot已确认送达', delivery_unknown: '投递结果未知', confirmed_sent: '本人对账为已发送', failed: '发送失败', superseded: '已合并到后续阶段', suppressed: '已停止发送', abandoned: '本人已放弃重发',
};
