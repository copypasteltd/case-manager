import type { Schemas } from './session';
export type SubscriptionInput = { product: string; category: string; start: string; trial: string; cycle: string; timezone: string; first: string; anchor: string; interval: string; renewal: string; method: string; reminders: string; notes: string; reason: string; prices: { from: string; to: string; currency: string; amount: string; plan: string }[] };
const limits = { product: 200, category: 50, start: 10, trial: 10, cycle: 10, timezone: 200, first: 10, anchor: 2, interval: 4, renewal: 10, method: 100, reminders: 100, notes: 20000, reason: 200 } as const;
export function subscriptionInput(s?: Schemas['Subscription']): SubscriptionInput {
  return { product: s?.product ?? '', category: s?.category ?? '', start: s?.start_date ?? '', trial: s?.trial_until ?? '', cycle: s?.cycle ?? '', timezone: s?.timezone ?? 'Asia/Hong_Kong', first: s?.first_charge_date ?? '', anchor: s?.anchor_day?.toString() ?? '', interval: s?.custom_interval_days?.toString() ?? '', renewal: s?.renewal_mode ?? '', method: s?.payment_method_label ?? '', reminders: s?.reminder_days.join(', ') ?? '', notes: s?.notes ?? '', reason: '', prices: s?.prices.map(p => ({ from: p.effective_from, to: p.effective_to ?? '', currency: p.currency, amount: p.amount, plan: p.plan })) ?? [] };
}
export function copySubscriptionInput(value: SubscriptionInput): SubscriptionInput | undefined {
  if (!value || !Array.isArray(value.prices) || value.prices.length > 100) return;
  const copy = {} as SubscriptionInput;
  for (const key of Object.keys(limits) as (keyof typeof limits)[]) { if (typeof value[key] !== 'string' || value[key].length > limits[key]) return; copy[key] = value[key]; }
  copy.prices = [];
  for (const price of value.prices) { const next = {} as SubscriptionInput['prices'][number]; for (const [key, limit] of Object.entries({ from: 10, to: 10, currency: 3, amount: 30, plan: 200 })) { const text = price?.[key as keyof typeof price]; if (typeof text !== 'string' || text.length > limit) return; next[key as keyof typeof next] = text; } copy.prices.push(next); }
  return copy;
}
export const subscriptionInputCharacters = (value: SubscriptionInput) => Object.values(value).reduce<number>((sum, v) => sum + (typeof v === 'string' ? v.length : v.reduce((n, p) => n + Object.values(p).reduce((a, t) => a + t.length, 0), 0)), 0);
export function subscriptionContent(input: SubscriptionInput, evidence: Pick<Schemas['SubscriptionContent'], 'supplier' | 'owner_id' | 'vendor_state' | 'vendor_checked_at' | 'vendor_evidence_version_ids'>): Schemas['SubscriptionContent'] {
  const raw = input.reminders.trim() ? input.reminders.split(',').map(s => s.trim()) : [], reminders = raw.map(Number);
  if (raw.some(t => !/^\d{1,2}$/.test(t)) || reminders.some(n => n > 90) || reminders.length > 10 || new Set(reminders).size !== reminders.length) throw new Error('提前提醒天数须为0至90，不可重复，最多10项，用英文逗号分隔。');
  if (!['', 'month', 'year', 'custom'].includes(input.cycle) || !['', 'automatic', 'manual'].includes(input.renewal)) throw new Error('请重新选择计费周期与续费方式。');
  function integer(text: string, max: number) { if (!text) return null; if (!/^\d+$/.test(text) || Number(text) < 1 || Number(text) > max) throw new Error('原始收费日或间隔天数无效。'); return Number(text); }
  return { product: input.product.trim(), category: input.category.trim(), start_date: input.start || null, trial_until: input.trial || null, cycle: input.cycle as Schemas['SubscriptionContent']['cycle'] || null, timezone: input.timezone, first_charge_date: input.first || null, anchor_day: integer(input.anchor, 31), custom_interval_days: integer(input.interval, 3660), renewal_mode: input.renewal as Schemas['SubscriptionContent']['renewal_mode'], payment_method_label: input.method, reminder_days: reminders, notes: input.notes, prices: input.prices.map(p => ({ effective_from: p.from, effective_to: p.to || null, currency: p.currency, amount: p.amount, plan: p.plan })), ...evidence };
}
export const subscriptionBase = (s: { version: number; current_revision_id: string; content_hash: string }, reason: string) => ({ expected_version: s.version, revision_id: s.current_revision_id, content_hash: s.content_hash, reason: reason.trim() });
