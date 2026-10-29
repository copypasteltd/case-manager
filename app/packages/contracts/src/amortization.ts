import type { components } from './api';

type S = components['schemas'];
export type AmortizationOverview = S['AmortizationOverview'];
export type AmortizationVersion = S['AmortizationVersion'];
export type AmortizationPreview = S['AmortizationPreview'];
export type AmortizationContent = S['AmortizationContent'];
export type AmortizationInput = S['AmortizationInput'];
export type AmortizationCost = Pick<S['Cost'], 'id' | 'version' | 'current_revision_id' | 'content_hash' | 'currency' | 'amount' | 'allocations'>;
export const amortizationMethods = [
  { value: 'monthly_equal', label: '所涉月份均分（首尾月同权）' },
  { value: 'daily_weighted', label: '按实际服务天数分配' },
] as const;
const fail = () => { throw new Error('摊销资料与原费用或所选版本不一致，请刷新后重新核对。'); };
const id = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/i.test(v);
const hash = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
const sequence = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) > 0;
const day = (v: unknown): v is string => typeof v === 'string' && /^\d{4}-\d\d-\d\d$/.test(v) && Number.isFinite(Date.parse(v)) && new Date(v + 'T00:00:00Z').toISOString().slice(0, 10) === v;
function units(v: unknown): bigint {
  if (typeof v !== 'string' || !/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(v)) return fail();
  const [whole, fraction = ''] = v.split('.');
  return BigInt(whole + fraction.padEnd(6, '0'));
}
export function amortizationInput(cost: AmortizationCost, planVersion: number, method: string, from: string, to: string, reason: string): AmortizationInput {
  if (!id(cost.id) || !sequence(cost.version) || !id(cost.current_revision_id) || !hash(cost.content_hash) ||
    !Number.isSafeInteger(planVersion) || planVersion < 0 || planVersion >= Number.MAX_SAFE_INTEGER ||
    !amortizationMethods.some(v => v.value === method) || !day(from) || !day(to) || from >= to ||
    Date.parse(to) - Date.parse(from) > 3660 * 86400000 || !reason.trim() || Array.from(reason.trim()).length > 200) fail();
  return { expected_version: cost.version, revision_id: cost.current_revision_id, content_hash: cost.content_hash,
    expected_plan_version: planVersion, method: method as AmortizationInput['method'], service_from: from, service_to: to, reason: reason.trim() };
}
export function checkedAmortizationContent(value: AmortizationContent, cost: AmortizationCost): AmortizationContent {
  if (!value || value.cost_id !== cost.id || !sequence(value.cost_version) || value.cost_version > cost.version ||
    value.cost_revision_id !== cost.current_revision_id || value.cost_sha256 !== cost.content_hash ||
    value.currency !== cost.currency || units(value.amount) !== units(cost.amount) || units(value.amount) <= 0n ||
    !Number.isInteger(value.exponent) || value.exponent < 0 || value.exponent > 6 ||
    !amortizationMethods.some(v => v.value === value.method) || value.rounding !== 'allocation_cumulative_half_up' ||
    !day(value.service_from) || !day(value.service_to) || value.service_from >= value.service_to ||
    !Array.isArray(value.periods) || value.periods.length < 1 || value.periods.length > 120) fail();
  const original = new Map<string, bigint>();
  for (const share of cost.allocations) {
    const key = share.project_id ?? 'company';
    original.set(key, (original.get(key) ?? 0n) + units(share.amount));
  }
  const scheduled = new Map<string, bigint>();
  // Legacy Taro targets lower exponentiation to Math.pow, which rejects BigInt.
  let quantum = 1n;
  for (let place = value.exponent; place < 6; place += 1) quantum *= 10n;
  let total = 0n, end = value.service_from;
  value.periods.forEach((p, index) => {
    if (p.sequence !== index + 1 || !day(p.service_from) || !day(p.service_to) || !day(p.recognized_on) ||
      p.service_from !== end || p.service_from >= p.service_to || new Date(Date.parse(p.service_to) - 86400000).toISOString().slice(0, 10) !== p.recognized_on ||
      !Array.isArray(p.shares) || p.shares.length > 100) fail();
    let shared = 0n;
    const seen = new Set<string>();
    for (const share of p.shares) {
      const key = share.project_id ?? 'company', amount = units(share.amount);
      if (share.project_id !== null && !id(share.project_id) || !original.has(key) || seen.has(key) || amount <= 0n || amount % quantum !== 0n) fail();
      seen.add(key); shared += amount; scheduled.set(key, (scheduled.get(key) ?? 0n) + amount);
    }
    const amount = units(p.amount);
    if (shared !== amount || amount % quantum !== 0n) fail();
    total += amount; end = p.service_to;
  });
  if (end !== value.service_to || total !== units(value.amount) || [...original].some(([key, amount]) => (scheduled.get(key) ?? 0n) !== amount)) fail();
  return value;
}
export function checkedAmortizationPreview(value: AmortizationPreview, cost: AmortizationCost, input: AmortizationInput): AmortizationPreview {
  if (!value || value.plan_version !== input.expected_plan_version + 1 || !hash(value.content_sha256)) fail();
  const c = checkedAmortizationContent(value.content, cost);
  if (c.cost_version !== input.expected_version || c.method !== input.method || c.service_from !== input.service_from || c.service_to !== input.service_to) fail();
  return value;
}
export function checkedAmortizationVersion(value: AmortizationVersion, cost: AmortizationCost, version?: number): AmortizationVersion {
  if (!value || !id(value.id) || value.cost_id !== cost.id || !sequence(value.version) || version !== undefined && value.version !== version ||
    !['active', 'stopped'].includes(value.state) || !hash(value.content_sha256) || !id(value.actor_id) ||
    !Number.isInteger(value.period_count) || value.period_count < 1 || value.period_count > 120 ||
    typeof value.reason !== 'string' || !value.reason.trim() || !Number.isFinite(Date.parse(value.created_at))) fail();
  if (value.content && checkedAmortizationContent(value.content, cost).periods.length !== value.period_count) fail();
  return value;
}
export function checkedAmortizationOverview(value: AmortizationOverview, cost: AmortizationCost): AmortizationOverview {
  if (!value || value.cost_id !== cost.id || !Number.isSafeInteger(value.version) || value.version < 0 || typeof value.can_manage !== 'boolean' ||
    (value.current !== null && (typeof value.current !== 'object' || !value.current)) ||
    (value.current === null) !== (value.version === 0)) fail();
  if (value.current) {
    checkedAmortizationVersion(value.current, cost, value.version);
    if (!value.current.content) fail();
  }
  return value;
}
