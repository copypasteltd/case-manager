import { MobileError, type Schemas } from './session';

export type FinancialKind = 'cost' | 'payment';
export type FinancialRecord = Schemas['Cost'] | Schemas['Payment'];
export type FinancialAction = 'create' | 'edit' | 'confirm' | 'reverse' | 'link' | 'record' | 'allocate' | 'release' | 'refundAllocate';
export const financialNames = { cost: '实际费用', payment: '资金记录' };
export const financialRoutes = { cost: '/costs', payment: '/payments' };
const limits = { title: 200, project: 36, classification: 20, source: 199, date: 19, currency: 3, amount: 30, category: 50, notes: 20000, reason: 200, direction: 20, method: 100, external: 200, effective: 19, fx_cost_amount: 30, fx_rate: 25, fx_source: 200 } as const;
export type FinancialInput = { [K in keyof typeof limits]: string } & { allocations: { project: string; amount: string }[] };
export type FinancialEvidence = { party: NonNullable<Schemas['Cost']['counterparty']> | null; files: string[]; employee: string | null; refund: string | null };
const idPattern = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const hashPattern = /^[a-f0-9]{64}$/;
function invalid(message: string): never { throw new MobileError('INVALID_INPUT', message); }
export function financialID(value: unknown): value is string { return typeof value === 'string' && idPattern.test(value); }
export function financialKind(value: unknown): FinancialKind { if (value === 'cost' || value === 'payment') return value; return invalid('请选择实际费用或资金记录。'); }
export function financialAction(kind: FinancialKind, value: unknown): FinancialAction {
  const actions = kind === 'cost' ? ['create', 'edit', 'confirm', 'reverse', 'link'] : ['create', 'edit', 'record', 'reverse', 'allocate', 'release', 'refundAllocate'];
  if (typeof value !== 'string' || !actions.includes(value)) return invalid('此类业务不支持所选操作。');
  return value as FinancialAction;
}
export function copyFinancialInput(value: unknown): FinancialInput | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return;
  const input = value as Record<string, unknown>, result: Record<string, unknown> = {};
  for (const [key, max] of Object.entries(limits)) {
    const current = input[key] === undefined && key.startsWith('fx_') ? '' : input[key];
    if (typeof current !== 'string' || current.length > max) return;
    result[key] = current;
  }
  if (!Array.isArray(input.allocations) || input.allocations.length > 100) return;
  const allocations: FinancialInput['allocations'] = [];
  for (const line of input.allocations) {
    if (!line || typeof line.project !== 'string' || line.project.length > 36 || typeof line.amount !== 'string' || line.amount.length > 30) return;
    allocations.push({ project: line.project, amount: line.amount });
  }
  return { ...result, allocations } as FinancialInput;
}
export function financialInputCharacters(value: FinancialInput) { return Object.keys(limits).reduce((n, key) => n + value[key as keyof typeof limits].length, 0) + value.allocations.reduce((n, v) => n + v.project.length + v.amount.length, 0); }
export function financialInput(kind: FinancialKind, value?: FinancialRecord, source = ''): FinancialInput {
  return { title: value?.title ?? '', project: value?.project_id ?? '', classification: value?.classification ?? 'internal', source: value?.source_key ?? source,
    date: value && 'occurred_on' in value ? value.occurred_on ?? '' : value && 'paid_at' in value ? hongKongInput(value.paid_at) : '',
    currency: value?.currency ?? '', amount: value?.amount ?? '', category: value?.category ?? '', notes: value?.notes ?? '', reason: '',
    direction: kind === 'payment' && value && 'direction' in value ? value.direction ?? '' : '', method: value && 'method_label' in value ? value.method_label : '',
    external: value && 'external_ref' in value ? value.external_ref : '', effective: '', fx_cost_amount: '', fx_rate: '', fx_source: '',
    allocations: value && 'allocations' in value ? value.allocations.map(line => ({ project: line.project_id ?? '', amount: line.amount })) : [] };
}
export function financialEvidence(value?: FinancialRecord): FinancialEvidence {
  return { party: value?.counterparty ? { ...value.counterparty } : null, files: [...value?.evidence_version_ids ?? []], employee: value && 'employee_id' in value ? value.employee_id : null, refund: value && 'refund_of_id' in value ? value.refund_of_id : null };
}
// Six decimal places match numeric(20,6). Never coerce amounts to Number.
export function financialUnits(value: string): bigint {
  if (!/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(value)) return invalid('金额须为最多14位整数、6位小数的非负十进制字符串。');
  const [whole, fraction = ''] = value.split('.'); return BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6, '0'));
}
export function positiveFinancialAmount(value: string) { if (financialUnits(value) <= 0n) return invalid('金额须大于零。'); return value; }
export function financialAmountEqual(left: string | null, right: string | null): boolean {
  if (left === null || right === null) return left === right;
  try { return financialUnits(left) === financialUnits(right); } catch { return false; }
}
function dateOnly(value: string) { const date = new Date(value + 'T00:00:00Z'); return /^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value; }
export function hongKongInput(value: string | null) {
  if (!value) return ''; const at = Date.parse(value); if (!Number.isFinite(at)) return '';
  return new Date(at + 8 * 3600000).toISOString().slice(0, 19);
}
export function fromHongKongInput(value: string) {
  const normalized = value.length === 16 ? value + ':00' : value;
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(normalized)) return invalid('请填写准确香港本地日期与时分秒。');
  const result = new Date(normalized + '+08:00');
  if (!Number.isFinite(result.getTime()) || hongKongInput(result.toISOString()) !== normalized) return invalid('香港日期或时间无效。');
  return result.toISOString();
}
export function financialBase(value: FinancialRecord, reason: string) {
  if (!financialID(value.id) || !Number.isSafeInteger(value.version) || value.version < 1 || !financialID(value.current_revision_id) || !hashPattern.test(value.content_hash) || !reason.trim() || [...reason.trim()].length > 200) return invalid('请固定准确业务版本、修订、摘要并说明操作依据。');
  return { expected_version: value.version, revision_id: value.current_revision_id, content_hash: value.content_hash, reason: reason.trim() };
}
function verifyInput(input: FinancialInput, evidence: FinancialEvidence) {
  if (!copyFinancialInput(input) || !input.title.trim() || !input.reason.trim()) return invalid('请填写业务标题及登记／修订原因。');
  if (input.project && !financialID(input.project) || !['internal', 'project_restricted', 'confidential'].includes(input.classification)) return invalid('请核对项目范围及资料分类。');
  if (input.currency && !/^[A-Z]{3}$/.test(input.currency) || input.amount && !input.currency) return invalid('请先填写准确三位原币，再输入金额。');
  if (input.amount) financialUnits(input.amount);
  if (evidence.party && (!financialID(evidence.party.party_id) || !financialID(evidence.party.revision_id)) || evidence.files.length > 20 || evidence.files.some(id => !financialID(id)) || new Set(evidence.files).size !== evidence.files.length) return invalid('请选择准确往来方修订及最多20份不重复的固定凭证。');
}
export function costContent(input: FinancialInput, evidence: FinancialEvidence): Schemas['CostContent'] {
  verifyInput(input, evidence);
  if (input.date && !dateOnly(input.date)) return invalid('费用发生日期无效。');
  const scopes = new Set<string>();
  const allocations = input.allocations.map(line => {
    if (line.project && !financialID(line.project) || scopes.has(line.project) || !input.currency) return invalid('分摊项目／公司不可重复，须先填写币种。');
    scopes.add(line.project); return { project_id: line.project || null, amount: positiveFinancialAmount(line.amount) };
  });
  return { title: input.title.trim(), occurred_on: input.date || null, currency: input.currency || null, amount: input.amount || null, category: input.category.trim(), counterparty: evidence.party, allocations, evidence_version_ids: [...evidence.files], notes: input.notes };
}
export function paymentContent(input: FinancialInput, evidence: FinancialEvidence): Schemas['PaymentContent'] {
  verifyInput(input, evidence);
  if (input.direction && !['outgoing', 'incoming', 'refund', 'transfer'].includes(input.direction) || evidence.employee && !financialID(evidence.employee) || evidence.refund && !financialID(evidence.refund)) return invalid('请核对方向、员工与退款原记录。');
  if (input.direction === 'transfer' && (evidence.party || evidence.employee) || evidence.party && evidence.employee || (input.direction === 'refund') !== Boolean(evidence.refund)) return invalid('转账不指定往来方；员工与供应方不可同时选择；退款须固定原付款。');
  return { title: input.title.trim(), direction: input.direction ? input.direction as Schemas['PaymentContent']['direction'] : null,
    paid_at: input.date ? fromHongKongInput(input.date) : null, currency: input.currency || null, amount: input.amount || null,
    category: input.category.trim(), counterparty: evidence.party, employee_id: evidence.employee, refund_of_id: evidence.refund,
    method_label: input.method.trim(), external_ref: input.external.trim(), evidence_version_ids: [...evidence.files], notes: input.notes };
}
export function financialDraftBody(kind: FinancialKind, input: FinancialInput, evidence: FinancialEvidence, value?: FinancialRecord) {
  const content = kind === 'cost' ? costContent(input, evidence) : paymentContent(input, evidence);
  if (value) return { ...content, ...financialBase(value, input.reason) };
  if (!/^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$/.test(input.source)) return invalid('请保留准确且永久唯一的来源标识。');
  return { ...content, project_id: input.project || null, classification: input.classification, source_key: input.source, reason: input.reason.trim() };
}
export function financialCan(kind: FinancialKind, action: FinancialAction, value: FinancialRecord | undefined, actions: string[]) {
  if (action === 'create') return actions.includes(kind === 'cost' ? 'cost.record' : 'payment.record');
  if (!value) return false;
  if (action === 'edit') return value.can_edit;
  if (action === 'reverse') return value.can_reverse;
  if (kind === 'cost' && 'can_confirm' in value) return action === 'confirm' ? value.can_confirm : action === 'link' && value.can_link;
  if (kind === 'payment' && 'can_record' in value) return action === 'record' ? value.can_record : ['allocate', 'release', 'refundAllocate'].includes(action) && value.can_allocate;
  return false;
}
function sameParty(left: NonNullable<Schemas['Cost']['counterparty']> | null, right: NonNullable<Schemas['Cost']['counterparty']> | null) { return left?.party_id === right?.party_id && left?.revision_id === right?.revision_id; }
function sameFiles(left: string[], right: string[]) { return left.length === right.length && [...left].sort().join(',') === [...right].sort().join(','); }
export function financialDraftReceipt(value: FinancialRecord, kind: FinancialKind, input: FinancialInput, evidence: FinancialEvidence, before?: FinancialRecord) {
  if (!financialRecordReceipt(value, kind) || value.state !== 'draft' || value.version !== (before ? before.version + 1 : 1) || before && (value.id !== before.id || value.current_revision_id === before.current_revision_id) || value.project_id !== (before ? before.project_id : input.project || null) || value.source_key !== (before?.source_key ?? input.source) || value.classification !== (before?.classification ?? input.classification)) return false;
  const content = kind === 'cost' ? costContent(input, evidence) : paymentContent(input, evidence);
  if (value.title !== content.title || value.currency !== content.currency || !financialAmountEqual(value.amount, content.amount) || value.category !== content.category || value.notes !== content.notes || !sameParty(value.counterparty, content.counterparty) || !sameFiles(value.evidence_version_ids, content.evidence_version_ids)) return false;
  if ('occurred_on' in content && 'occurred_on' in value) return value.occurred_on === content.occurred_on && value.allocations.length === content.allocations.length && value.allocations.every((line, i) => line.project_id === content.allocations[i].project_id && financialAmountEqual(line.amount, content.allocations[i].amount));
  if ('paid_at' in content && 'paid_at' in value) return value.direction === content.direction && value.employee_id === content.employee_id && value.refund_of_id === content.refund_of_id && value.method_label === content.method_label && value.external_ref === content.external_ref && (value.paid_at === null || content.paid_at === null ? value.paid_at === content.paid_at : Date.parse(value.paid_at) === Date.parse(content.paid_at));
  return false;
}
export function financialRecordReceipt(value: unknown, kind: FinancialKind): value is FinancialRecord {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const v = value as FinancialRecord;
  return financialID(v.id) && Number.isSafeInteger(v.version) && v.version > 0 && financialID(v.current_revision_id) && hashPattern.test(v.content_hash) && typeof v.title === 'string' && Array.isArray(v.evidence_version_ids) && (kind === 'cost' ? 'allocations' in v && Array.isArray(v.allocations) && 'can_confirm' in v : 'direction' in v && 'can_record' in v);
}
export function financialTransitionReceipt(value: FinancialRecord, kind: FinancialKind, before: FinancialRecord, action: FinancialAction) {
  if (!financialRecordReceipt(value, kind) || value.id !== before.id || value.version !== before.version + 1 || value.source_key !== before.source_key || value.project_id !== before.project_id || value.classification !== before.classification || value.content_hash !== before.content_hash || !financialAmountEqual(value.amount, before.amount) || value.currency !== before.currency) return false;
  if (action === 'reverse') return value.state === 'reversed' && financialID(value.reversal_id);
  if (action === 'confirm') return value.state === 'confirmed';
  if (action === 'record') return value.state === 'recorded';
  return value.state === before.state;
}
export function financialReversalReceipt(value: unknown, kind: FinancialKind, before: FinancialRecord, effective: string) {
  if (!financialRecordReceipt(value, kind) || value.id === before.id || value.version !== 1 || value.reverses_id !== before.id || value.project_id !== before.project_id || value.classification !== before.classification || value.currency !== before.currency || !financialAmountEqual(value.amount, before.amount) || !sameParty(value.counterparty, before.counterparty) || !sameFiles(value.evidence_version_ids, before.evidence_version_ids)) return false;
  if (kind === 'cost' && 'occurred_on' in value && 'occurred_on' in before) return value.entry_kind === 'reversal' && value.state === 'confirmed' && value.occurred_on === effective && value.allocations.length === before.allocations.length && value.allocations.every((line, i) => line.project_id === before.allocations[i].project_id && financialAmountEqual(line.amount, before.allocations[i].amount));
  if (kind === 'payment' && 'paid_at' in value && 'paid_at' in before) return value.record_kind === 'reversal' && value.state === 'recorded' && value.direction === before.direction && value.employee_id === before.employee_id && value.refund_of_id === before.refund_of_id && value.paid_at !== null && Date.parse(value.paid_at) === Date.parse(fromHongKongInput(effective));
  return false;
}
export function sourceEligible(value: Schemas['Spend'] | Schemas['Contract'], cost: Schemas['Cost'], project: string) {
  return value.project_id === (project || null) && value.currency === cost.currency && value.approval_state === 'approved' && (!('direction' in value) || value.lifecycle === 'effective' && 'direction' in value && value.direction === 'payable');
}
export function settlementEligible(cost: Schemas['Cost'], payment: Schemas['Payment'], claim?: Schemas['Spend'], line?: string, allowFX = false) {
  const supplier = claim?.lines.find(v => v.id === line)?.supplier?.party_id ?? null;
  return cost.state === 'confirmed' && cost.entry_kind === 'expense' && (allowFX || cost.currency === payment.currency) && cost.counterparty?.party_id === (payment.employee_id ? supplier : payment.counterparty?.party_id) && (!claim || claim.approval_state === 'approved' && claim.creator_id === payment.employee_id && claim.currency === cost.currency && cost.links.some(v => v.source_type === 'claim' && v.source_id === claim.id && v.source_revision_id === claim.approved_revision_id));
}
export function financialAllocation(payment: Schemas['Payment'], cost: Schemas['Cost'], reason: string, amount: string, claim?: Schemas['Spend'], line?: string, fx?: Schemas['PaymentFXInput']): Schemas['PaymentAllocate'] {
  if (payment.state !== 'recorded' || payment.direction !== 'outgoing' || payment.record_kind !== 'payment') return invalid('只有已登记原支出资金可分配费用结算。');
  financialBase(cost, reason);
  if (!settlementEligible(cost, payment, claim, line, !!fx) || payment.employee_id && (!claim || !line || !claim.lines.some(v => v.id === line && v.funding === 'employee_advance'))) return invalid('请选择同供应方的已确认费用；跨币须固定人工汇率，员工结算须固定同费用币种的批准垫付明细。');
  positiveFinancialAmount(amount);
  if (fx && (cost.currency === payment.currency || !fx.acknowledged || !fx.source.trim() || [...fx.source.trim()].length > 200 || !/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,10})?$/.test(fx.rate) || !/[1-9]/.test(fx.rate))) return invalid('请核对实际跨币汇率、凭证来源和确认声明。');
  if (fx) positiveFinancialAmount(fx.cost_amount);
  return { ...financialBase(payment, reason), cost_id: cost.id, cost_version: cost.version, cost_revision_id: cost.current_revision_id, cost_content_hash: cost.content_hash, amount, claim_id: claim?.id ?? null, claim_revision_id: claim?.approved_revision_id ?? null, claim_line_id: line || null, ...(fx ? { fx } : {}) };
}
