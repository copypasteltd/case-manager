import { sha256 } from '@noble/hashes/sha2.js';
import { MobileError, type Schemas } from './session';
import { utf8Bytes } from './encoding';

export type BusinessKind = 'contract' | 'purchase' | 'claim';
export type Business = Schemas['Contract'] | Schemas['Spend'];
export const businessNames: Record<BusinessKind, string> = { contract: '合同', purchase: '采购', claim: '报销' };
export const businessRoutes: Record<BusinessKind, string> = { contract: '/contracts', purchase: '/purchases', claim: '/claims' };
export function businessKind(value: unknown): BusinessKind {
  if (value === 'contract' || value === 'purchase' || value === 'claim') return value;
  throw new Error('Unknown business kind');
}

// Editable strings only. Fixed files, parties, confirmations, credentials,
// server responses and submitted command bodies never enter this structure.
const scalarLimits = {
  title: 200, category: 50, purpose: 4000, owner: 36, project: 36, parent: 36,
  classification: 20, currency: 3, reason: 200, notes: 20000, direction: 20,
  contractType: 200, amount: 30, tax: 30, start: 10, end: 10,
  renewal: 20000, termination: 20000, scheduleMode: 20, scheduleReason: 20000,
  pricing: 30, rounding: 20, delivery: 10, periodStart: 10, periodEnd: 10, budget: 500,
} as const;
const lineLimits = {
  id: 36, description: 1000, quantity: 30, unit: 50, price: 30,
  amount: 30, tax: 30, discount: 30, date: 10, category: 50,
  funding: 20, missing: 2000, duplicate: 2000,
} as const;
const planLimits = { id: 36, title: 200, amount: 30, due: 10, conditions: 20000 } as const;
export type BusinessLine = Record<keyof typeof lineLimits, string>;
export type BusinessPlan = Record<keyof typeof planLimits, string>;
export type BusinessInput = Record<keyof typeof scalarLimits, string> & { kind: BusinessKind; lines: BusinessLine[]; plans: BusinessPlan[] };
export type BusinessEvidence = {
  unresolved: boolean;
  counterparty: Schemas['BusinessPartyReference'] | null;
  body: Schemas['BusinessFileReference'] | null;
  candidates: Schemas['SpendSupplierEvidence'][];
  quotes: Schemas['BusinessFileReference'][];
  lineSuppliers: Record<string, Schemas['SpendSupplierEvidence'] | undefined>;
  receipts: Record<string, Schemas['BusinessFileReference'][] | undefined>;
};

export const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function businessReceipt(value: unknown, kind: BusinessKind, expectedID?: string, minimumVersion = 1): value is Business {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const item = value as Business;
  return uuidPattern.test(item.id) && (!expectedID || item.id === expectedID) && Number.isSafeInteger(item.version) && item.version >= minimumVersion
    && uuidPattern.test(item.current_revision_id) && /^[a-f0-9]{64}$/.test(item.content_hash) && typeof item.can_edit === 'boolean'
    && (kind === 'contract' ? 'contract_no' in item && typeof item.contract_no === 'string' : 'kind' in item && item.kind === kind);
}
export function requestReceipt(value: unknown, expected: { kind: string; businessID: string; state: Schemas['Request']['state']; id?: string; minimumVersion?: number; revisionID?: string; previousRevisionID?: string; businessRevisionID?: string }): value is Schemas['Request'] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const item = value as Schemas['Request'];
  return uuidPattern.test(item.id) && (!expected.id || item.id === expected.id) && item.kind === expected.kind && item.business_id === expected.businessID
    && Number.isSafeInteger(item.version) && item.version >= (expected.minimumVersion ?? 1) && item.state === expected.state
    && uuidPattern.test(item.current_revision_id) && (!expected.revisionID || item.current_revision_id === expected.revisionID) && (!expected.previousRevisionID || item.current_revision_id !== expected.previousRevisionID) && /^[a-f0-9]{64}$/.test(item.content_hash)
    && Boolean(item.payload && typeof item.payload === 'object' && !Array.isArray(item.payload) && item.payload.kind === expected.kind && item.payload.business_id === expected.businessID
      && uuidPattern.test(item.payload.business_revision_id) && (!expected.businessRevisionID || item.payload.business_revision_id === expected.businessRevisionID))
    && typeof item.can_submit === 'boolean' && typeof item.can_revise === 'boolean' && typeof item.can_withdraw === 'boolean';
}
export function formID(publicIntent: string): string {
  // Independent public server request nonce + session counter, not a token.
  const bytes = sha256(utf8Bytes(publicIntent));
  bytes[6] = (bytes[6] & 15) | 64; bytes[8] = (bytes[8] & 63) | 128;
  const hex = Array.from(bytes.slice(0, 16), byte => byte.toString(16).padStart(2, '0')).join('');
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)].join('-');
}
function strings<T extends Record<string, number>>(input: unknown, limits: T): Record<keyof T, string> | undefined {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return;
  const result = {} as Record<keyof T, string>;
  for (const name of Object.keys(limits) as (keyof T)[]) {
    const value = (input as Record<keyof T, unknown>)[name];
    if (typeof value !== 'string' || value.length > limits[name]) return;
    result[name] = value;
  }
  return result;
}
export function copyBusinessInput(input: unknown): BusinessInput | undefined {
  const fields = strings(input, scalarLimits); if (!fields) return;
  const value = input as BusinessInput;
  if (!['contract', 'purchase', 'claim'].includes(value.kind) || !Array.isArray(value.lines) || value.lines.length > 100 || !Array.isArray(value.plans) || value.plans.length > 100) return;
  if (!['internal', 'project_restricted', 'confidential'].includes(value.classification) || !['', 'receivable', 'payable', 'none'].includes(value.direction) || !['', 'scheduled', 'no_payment'].includes(value.scheduleMode)) return;
  if (!['', 'net_plus_tax', 'gross_includes_tax'].includes(value.pricing) || !['', 'half_up', 'half_even'].includes(value.rounding)) return;
  for (const name of ['owner', 'project', 'parent'] as const) if (value[name] && !uuidPattern.test(value[name])) return;
  const lines = value.lines.map(line => strings(line, lineLimits)), plans = value.plans.map(plan => strings(plan, planLimits));
  if (lines.some(line => !line || !uuidPattern.test(line.id)) || plans.some(plan => !plan || !uuidPattern.test(plan.id))) return;
  return { ...fields, kind: value.kind, lines: lines as BusinessLine[], plans: plans as BusinessPlan[] };
}
export function businessInputCharacters(value: BusinessInput): number {
  return value.kind.length + Object.keys(scalarLimits).reduce((n, key) => n + value[key as keyof typeof scalarLimits].length, 0)
    + [...value.lines, ...value.plans].reduce((n, row) => n + Object.values(row).reduce((sum, text) => sum + text.length, 0), 0);
}
export function newBusinessLine(id: string): BusinessLine {
  return { id, description: '', quantity: '', unit: '', price: '', amount: '', tax: '', discount: '', date: '', category: '', funding: '', missing: '', duplicate: '' };
}
export function newBusinessPlan(id: string): BusinessPlan { return { id, title: '', amount: '', due: '', conditions: '' }; }
export function businessInput(kind: BusinessKind, value?: Business, owner = ''): BusinessInput {
  const result: BusinessInput = {
    kind, title: value?.title ?? '', category: value?.category ?? '', purpose: '', owner: value?.owner_id ?? owner,
    project: value?.project_id ?? '', parent: '', classification: value?.classification ?? 'internal', currency: value?.currency ?? '',
    reason: '', notes: value?.notes ?? '', direction: '', contractType: '', amount: '', tax: '', start: '', end: '',
    renewal: '', termination: '', scheduleMode: '', scheduleReason: '', pricing: '', rounding: '', delivery: '',
    periodStart: '', periodEnd: '', budget: '', lines: [], plans: [],
  };
  if (!value) return result;
  if ('contract_no' in value) Object.assign(result, {
    parent: value.parent_contract_id ?? '', direction: value.direction ?? '', contractType: value.type ?? '', amount: value.amount ?? '',
    tax: value.tax_amount ?? '', start: value.start_date ?? '', end: value.end_date ?? '', renewal: value.renewal_terms, termination: value.termination_terms,
    scheduleMode: value.payment_schedule?.mode ?? '', scheduleReason: value.payment_schedule?.reason ?? '',
    plans: value.payment_schedule?.items.map(plan => ({ id: plan.id, title: plan.title, amount: plan.amount ?? '', due: plan.due_date ?? '', conditions: plan.conditions })) ?? [],
  });
  else Object.assign(result, {
    purpose: value.purpose, pricing: value.pricing_basis, rounding: value.rounding_mode, delivery: value.expected_delivery_date ?? '',
    periodStart: value.period_start ?? '', periodEnd: value.period_end ?? '', budget: value.budget_reference,
    lines: value.lines.map(line => ({ id: line.id, description: line.description, quantity: line.quantity ?? '', unit: line.unit,
      price: line.unit_price ?? '', amount: line.declared_amount ?? '', tax: line.tax_amount ?? '', discount: line.discount_amount ?? '',
      date: line.expense_date ?? '', category: line.category, funding: line.funding, missing: line.missing_receipt_reason, duplicate: line.duplicate_receipt_reason })),
  });
  return result;
}
export function businessEvidence(value?: Business): BusinessEvidence {
  if (!value) return { unresolved: false, counterparty: null, body: null, candidates: [], quotes: [], lineSuppliers: {}, receipts: {} };
  if ('contract_no' in value) return { ...businessEvidence(), counterparty: value.counterparty, body: value.draft_file, unresolved: !!((value.counterparty_id && !value.counterparty) || (value.draft_file_version_id && !value.draft_file)) };
  return {
    ...businessEvidence(), unresolved: value.supplier_candidates.some(ref => !value.suppliers.some(p => p.party_id === ref.party_id && p.revision_id === ref.revision_id)) || value.quote_version_ids.some(id => !value.attachments.some(file => file.version_id === id)) || value.lines.some(line => line.receipt_version_ids.some(id => !value.attachments.some(file => file.version_id === id)) || !!(line.supplier && !value.suppliers.some(p => p.party_id === line.supplier!.party_id && p.revision_id === line.supplier!.revision_id))),
    candidates: value.supplier_candidates.map(ref => value.suppliers.find(p => p.party_id === ref.party_id && p.revision_id === ref.revision_id)).filter((p): p is Schemas['SpendSupplierEvidence'] => !!p),
    quotes: value.attachments.filter(file => value.quote_version_ids.includes(file.version_id)),
    lineSuppliers: Object.fromEntries(value.lines.map(line => [line.id, line.supplier ? value.suppliers.find(p => p.party_id === line.supplier!.party_id && p.revision_id === line.supplier!.revision_id) : undefined])),
    receipts: Object.fromEntries(value.lines.map(line => [line.id, value.attachments.filter(file => line.receipt_version_ids.includes(file.version_id))])),
  };
}

export type InputIssue = { path: string; label: string; message: string };
export function serverBusinessIssues(error: unknown, input: BusinessInput): InputIssue[] {
  if (!(error instanceof MobileError) || ![400, 422].includes(error.status)) return [];
  const mapped: Record<string, [string, string]> = {
    title: ['title', '标题'], category: ['category', '业务类别'], reason: ['reason', '保存依据'], currency: ['currency', '币种'], amount: ['amount', '合同总额'],
    tax_amount: input.kind === 'contract' ? ['tax', '其中税额'] : ['lines', '明细税额'], owner_id: ['owner', '负责人'], project_id: ['project', '关联项目'], classification: ['project', '资料范围'],
    start_date: ['start', '起始日期'], end_date: ['end', '结束日期'], period_start: ['periodStart', '费用期间开始'], period_end: ['periodEnd', '费用期间结束'], expected_delivery_date: ['delivery', '预计交付日期'],
    pricing_basis: ['pricing', '计价方式'], rounding_mode: ['rounding', '舍入规则'], payment_schedule: ['schedule', '收付款计划'], lines: ['lines', '明细'],
    counterparty_id: ['contract-evidence', '往来方依据'], counterparty_revision_id: ['contract-evidence', '往来方依据'], draft_file_version_id: ['contract-evidence', '正文依据'],
  };
  return error.details.map(detail => {
    const location = mapped[detail.field];
    return location ? { path: location[0], label: location[1], message: detail.safe_message } : { path: 'save', label: '保存资料', message: detail.safe_message };
  });
}
export function businessIssues(input: BusinessInput): InputIssue[] {
  const issues: InputIssue[] = [], add = (path: string, label: string, message: string) => issues.push({ path, label, message });
  if (!input.title.trim()) add('title', '标题', '请填写标题。');
  if (!input.reason.trim()) add('reason', '保存依据', '请说明本次创建或修改的依据。');
  if (input.currency && !/^[A-Z]{3}$/.test(input.currency)) add('currency', '币种', '请填写三位大写币种，例如 HKD。');
  if (input.classification === 'project_restricted' && !input.project) add('project', '关联项目', '项目受限资料须选择项目。');
  function money(value: string, path: string, label: string, positive = false) {
    if (value && (!/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(value) || (positive && !/[1-9]/.test(value)))) add(path, label, positive ? '请输入大于零的十进制数量，最多六位小数。' : '请输入非负十进制数，最多六位小数；不接受科学计数法。');
  }
  function day(value: string, path: string, label: string) {
    if (!value) return;
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
    if (!match) { add(path, label, '请按 YYYY-MM-DD 填写有效日期。'); return; }
    const [year, month, date] = match.slice(1).map(Number), leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
    if (year < 1 || month < 1 || month > 12 || date < 1 || date > [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1]) add(path, label, '请填写实际存在的日历日期。');
  }
  if (input.kind === 'contract') {
    money(input.amount, 'amount', '合同总额'); money(input.tax, 'tax', '其中税额'); day(input.start, 'start', '起始日期'); day(input.end, 'end', '结束日期');
    if (input.start && input.end && input.end < input.start) add('end', '结束日期', '结束日期不能早于起始日期。');
    input.plans.forEach((plan, index) => { money(plan.amount, 'plan-' + plan.id + '-amount', '节点' + (index + 1) + '金额'); day(plan.due, 'plan-' + plan.id + '-due', '节点' + (index + 1) + '到期日'); if (!plan.title.trim()) add('plan-' + plan.id + '-title', '节点' + (index + 1) + '名称', '已添加的节点须填写名称。'); });
    if (input.scheduleMode === 'no_payment' && !input.scheduleReason.trim()) add('scheduleReason', '无收付款义务依据', '请填写明确无收付款义务的依据。');
  } else {
    day(input.delivery, 'delivery', '预计交付日期'); day(input.periodStart, 'periodStart', '费用期间开始'); day(input.periodEnd, 'periodEnd', '费用期间结束');
    if (input.periodStart && input.periodEnd && input.periodEnd < input.periodStart) add('periodEnd', '费用期间结束', '结束日期不能早于开始日期。');
    input.lines.forEach((line, index) => {
      const prefix = 'line-' + line.id + '-';
      for (const name of ['quantity', 'price', 'amount', 'tax', 'discount'] as const) money(line[name], prefix + name, '明细' + (index + 1) + ({ quantity: '数量', price: '单价', amount: '费用', tax: '税额', discount: '折扣' }[name]), name === 'quantity');
      day(line.date, prefix + 'date', '明细' + (index + 1) + '日期');
    });
  }
  return issues;
}
const nullable = (text: string) => text === '' ? null : text;
export function contractContent(input: BusinessInput, evidence: BusinessEvidence) {
  return {
    title: input.title.trim(), category: input.category, direction: nullable(input.direction) as Schemas['Contract']['direction'],
    type: nullable(input.contractType), owner_id: nullable(input.owner), currency: nullable(input.currency), amount: nullable(input.amount), tax_amount: nullable(input.tax),
    start_date: nullable(input.start), end_date: nullable(input.end), renewal_terms: input.renewal, termination_terms: input.termination, notes: input.notes,
    counterparty_id: evidence.counterparty?.id ?? null, counterparty_revision_id: evidence.counterparty?.revision_id ?? null, draft_file_version_id: evidence.body?.version_id ?? null,
    payment_schedule: input.scheduleMode ? { mode: input.scheduleMode as Schemas['ContractPaymentSchedule']['mode'], reason: input.scheduleReason,
      items: input.scheduleMode === 'scheduled' ? input.plans.map(plan => ({ id: plan.id, title: plan.title.trim(), amount: nullable(plan.amount), due_date: nullable(plan.due), conditions: plan.conditions })) : [] } : null,
  };
}
export function spendContent(input: BusinessInput, evidence: BusinessEvidence): Schemas['SpendContent'] {
  return {
    title: input.title.trim(), category: input.category, purpose: input.purpose, owner_id: nullable(input.owner), currency: nullable(input.currency),
    pricing_basis: input.pricing as Schemas['Spend']['pricing_basis'], rounding_mode: input.rounding as Schemas['Spend']['rounding_mode'],
    expected_delivery_date: nullable(input.delivery), period_start: nullable(input.periodStart), period_end: nullable(input.periodEnd), budget_reference: input.budget, notes: input.notes,
    supplier_candidates: evidence.candidates.map(p => ({ party_id: p.party_id, revision_id: p.revision_id })), quote_version_ids: evidence.quotes.map(file => file.version_id),
    lines: input.lines.map(line => ({ id: line.id, description: line.description, quantity: nullable(line.quantity), unit: line.unit, unit_price: nullable(line.price),
      declared_amount: nullable(line.amount), tax_amount: nullable(line.tax), discount_amount: nullable(line.discount), expense_date: nullable(line.date), category: line.category,
      supplier: evidence.lineSuppliers[line.id] ? { party_id: evidence.lineSuppliers[line.id]!.party_id, revision_id: evidence.lineSuppliers[line.id]!.revision_id } : null,
      receipt_version_ids: (evidence.receipts[line.id] ?? []).map(file => file.version_id), missing_receipt_reason: line.missing, duplicate_receipt_reason: line.duplicate, funding: line.funding as Schemas['SpendLine']['funding'] })),
  };
}

export function currentBusinessPayload(kind: BusinessKind, value: Business): Schemas['ContractRequestPayload'] | Schemas['SpendRequestPayload'] {
  const input = businessInput(kind, value), evidence = businessEvidence(value);
  const shared = { business_id: value.id, business_revision_id: value.current_revision_id, business_content_hash: value.content_hash, project_id: value.project_id, classification: value.classification };
  if ('contract_no' in value) return { ...shared, kind: 'contract', contract_no: value.contract_no, business_payload_schema_version: value.payload_schema_version, content: { ...contractContent(input, evidence), counterparty_id: value.counterparty_id, counterparty_revision_id: value.counterparty_revision_id, draft_file_version_id: value.draft_file_version_id }, counterparty: value.counterparty, attachments: value.draft_file ? [value.draft_file] : [] };
  return { ...shared, kind: value.kind, document_no: value.document_no, business_payload_schema_version: 1, content: { ...spendContent(input, evidence), supplier_candidates: value.supplier_candidates, quote_version_ids: value.quote_version_ids, lines: value.lines }, amount: value.amount, employee_reimbursement_amount: value.employee_reimbursement_amount, suppliers: value.suppliers, attachments: value.attachments, line_values: value.line_values };
}
