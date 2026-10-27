import test from 'node:test';
import assert from 'node:assert/strict';
import { MemoryDrafts } from './drafts';
import type { Schemas } from './session';
import { copyFinancialInput, costContent, financialAllocation, financialAmountEqual, financialBase, financialCan, financialDraftBody, financialDraftReceipt, financialEvidence, financialInput, financialReversalReceipt, financialTransitionReceipt, financialUnits, fromHongKongInput, hongKongInput, paymentContent, settlementEligible, sourceEligible } from './financial-input';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const supplier = { party_id: id(10), revision_id: id(11) };
const file: Schemas['BusinessFileReference'] = { file_id: id(20), version_id: id(21), seq: 1, name: '合成凭证.txt', size_bytes: '100', sha256: 'a'.repeat(64), media_type: 'text/plain', scan_state: 'clean', scan_waiver_ref: null };
const cost: Schemas['Cost'] = { id: id(1), version: 2, created_at: '2025-01-01T00:00:00Z', updated_at: '2025-01-02T00:00:00Z',
  title: '合成实际费用', occurred_on: '2025-01-01', currency: 'HKD', amount: '100.000000', category: 'software', counterparty: supplier,
  allocations: [{ project_id: id(3), amount: '60.000000' }, { project_id: null, amount: '40.000000' }], evidence_version_ids: [file.version_id], notes: '固定事实',
  source_key: 'manual:cost-reference-1', project_id: id(3), creator_id: id(4), classification: 'project_restricted', entry_kind: 'expense', base_currency: null, base_amount: null,
  state: 'confirmed', reverses_id: null, reversal_id: null, current_revision_id: id(2), content_hash: 'b'.repeat(64), counterparty_evidence: { ...supplier, name: '合成供应方' },
  attachments: [file], links: [], paid_amount: '0.000000', outstanding_amount: '100.000000', settlement_state: 'unsettled', settlement_visibility: 'complete', can_edit: false, can_confirm: false, can_reverse: true, can_link: true };
const payment: Schemas['Payment'] = { id: id(30), version: 2, created_at: '2025-01-01T00:00:00Z', updated_at: '2025-01-02T00:00:00Z',
  title: '合成实际资金', direction: 'outgoing', paid_at: '2025-01-31T18:00:00Z', currency: 'HKD', amount: '100.000000', category: 'software', counterparty: supplier,
  employee_id: null, refund_of_id: null, method_label: '银行转账', external_ref: 'sample-1', evidence_version_ids: [file.version_id], notes: '', source_key: 'manual:cash-reference-1', project_id: id(3), creator_id: id(4),
  classification: 'project_restricted', record_kind: 'payment', state: 'recorded', reverses_id: null, reversal_id: null, current_revision_id: id(31), content_hash: 'c'.repeat(64), paid_on: '2025-02-01',
  counterparty_evidence: cost.counterparty_evidence, attachments: [file], effective_allocated_amount: '0.000000', refunded_amount: '0.000000', contract_return_reserved_amount: '0.000000', unallocated_amount: '100.000000', can_edit: false, can_record: false, can_allocate: true, can_reverse: true };

test('money retains exact fractions and amounts beyond JavaScript safe integer units', () => {
  assert.equal(financialUnits('99999999999999.999999'), 99999999999999999999n);
  assert.equal(financialUnits('0.1') + financialUnits('0.2'), financialUnits('0.3'));
  assert.equal(financialAmountEqual('100', '100.000000'), true);
  assert.equal(financialAmountEqual('0.100001', '0.1'), false);
  for (const v of ['1e2', '-1', '01', 'NaN', '1.0000001', '100000000000000']) assert.throws(() => financialUnits(v));
});
test('Hong Kong input preserves the business day across UTC midnight and rejects invalid calendar times', () => {
  assert.equal(fromHongKongInput('2025-02-01T02:00'), '2025-01-31T18:00:00.000Z');
  assert.equal(hongKongInput('2025-01-31T18:00:00Z'), '2025-02-01T02:00:00');
  assert.equal(fromHongKongInput('2024-02-29T23:59:59'), '2024-02-29T15:59:59.000Z');
  for (const v of ['2025-02-29T02:00:00', '2025-01-32T02:00:00', '2025-01-01T24:00:00', '2025-01-01', '2025-01-01T02:00:00Z']) assert.throws(() => fromHongKongInput(v));
});
test('partial drafts preserve null facts and permanent source keys without inventing currency or payment', () => {
  const input = { ...financialInput('cost', undefined, 'manual:original'), title: '  暂缺资料  ', reason: '  整理  ' };
  const body = financialDraftBody('cost', input, financialEvidence());
  assert.equal(body.title, '暂缺资料'); assert.equal(body.amount, null); assert.equal(body.currency, null);
  assert.ok('source_key' in body); assert.equal(body.source_key, 'manual:original');
  assert.ok('allocations' in body); assert.deepEqual(body.allocations, []); assert.deepEqual(body.evidence_version_ids, []);
  assert.throws(() => financialDraftBody('cost', { ...input, source: 'not-a-source' }, financialEvidence()));
});
test('unreadable original fixed references remain in edits and arrays are independently copied', () => {
  const original = { ...cost, state: 'draft' as const, evidence_version_ids: [id(21), id(22)], attachments: [file] };
  const input = { ...financialInput('cost', original), reason: '保留原凭证' }, evidence = financialEvidence(original);
  const body = costContent(input, evidence); assert.deepEqual(body.evidence_version_ids, [id(21), id(22)]);
  body.evidence_version_ids.pop(); assert.equal(evidence.files.length, 2); assert.equal(original.evidence_version_ids.length, 2);
});
test('split scopes cannot duplicate company/project or accept invalid amounts and dates', () => {
  const input = { ...financialInput('cost', cost), reason: '核对' };
  assert.equal(costContent(input, financialEvidence(cost)).allocations.length, 2);
  assert.throws(() => costContent({ ...input, allocations: [{ project: '', amount: '50' }, { project: '', amount: '50' }] }, financialEvidence(cost)));
  assert.throws(() => costContent({ ...input, allocations: [{ project: id(3), amount: '0' }] }, financialEvidence(cost)));
  assert.throws(() => costContent({ ...input, date: '2025-02-30' }, financialEvidence(cost)));
  assert.throws(() => costContent({ ...input, currency: '' }, financialEvidence(cost)));
});
test('transfers, employee payees and refund references remain distinct', () => {
  const input = { ...financialInput('payment', payment), reason: '登记' }, evidence = financialEvidence(payment);
  assert.throws(() => paymentContent({ ...input, direction: 'transfer' }, evidence));
  assert.equal(paymentContent({ ...input, direction: 'transfer' }, { ...evidence, party: null }).counterparty, null);
  assert.throws(() => paymentContent(input, { ...evidence, employee: id(9) }));
  assert.throws(() => paymentContent({ ...input, direction: 'refund' }, evidence));
  assert.throws(() => paymentContent(input, { ...evidence, refund: id(99) }));
  assert.equal(paymentContent({ ...input, direction: 'refund' }, { ...evidence, refund: id(99) }).refund_of_id, id(99));
});
test('financial memory drafts retain editable strings, exclude evidence/acknowledgments and bind the exact revision', () => {
  let now = 100, authenticated = true;
  const store = new MemoryDrafts(() => ({ authenticated, epoch: 1 }), () => now);
  const target = { kind: 'financial-edit' as const, id: cost.id, version: cost.version, revision: cost.current_revision_id, hash: cost.content_hash };
  const authority = { userID: id(4), epoch: 1, authorizationVersion: 1 };
  const handle = store.open(authority, target)!;
  const input = { ...financialInput('cost', cost), reason: '保留文字' };
  assert.equal(handle.write({ reason: '', action: 'save', financial: Object.assign(input, { party: supplier, files: [file], mfa: '000000', acknowledged: true }) }), true);
  const restored = handle.read()!.financial!;
  assert.equal(restored.source, cost.source_key); assert.equal('mfa' in restored, false); assert.equal('files' in restored, false); assert.equal('party' in restored, false);
  restored.allocations[0].amount = '1'; assert.equal(handle.read()!.financial!.allocations[0].amount, '60.000000');
  assert.equal(store.open(authority, { ...target, hash: 'd'.repeat(64) })!.read(), undefined);
  now += 15 * 60000; assert.equal(handle.read(), undefined);
  handle.write({ reason: '', action: 'save', financial: input }); authenticated = false; assert.equal(handle.read(), undefined);
});
test('draft copy caps array/scalar sizes and cannot serialize an arbitrary server response', () => {
  const input = financialInput('cost', cost);
  assert.equal(copyFinancialInput({ ...input, notes: 'x'.repeat(20001) }), undefined);
  assert.equal(copyFinancialInput({ ...input, allocations: Array.from({ length: 101 }, () => ({ project: '', amount: '1' })) }), undefined);
  assert.equal(copyFinancialInput(cost), undefined);
  assert.equal(copyFinancialInput({ ...input, party: supplier })!.source, input.source);
});
test('draft success rejects wrong record, source, scope, currency, revision version or amount', () => {
  const before = { ...cost, state: 'draft' as const }, input = { ...financialInput('cost', before), reason: '修订' }, evidence = financialEvidence(before);
  const after = { ...before, version: 3, current_revision_id: id(90) };
  assert.equal(financialDraftReceipt(after, 'cost', input, evidence, before), true);
  for (const change of [{ id: id(99) }, { source_key: 'manual:wrong' }, { project_id: null }, { currency: 'USD' }, { version: 2 }, { amount: '101' }, { evidence_version_ids: [] }]) assert.equal(financialDraftReceipt({ ...after, ...change }, 'cost', input, evidence, before), false);
});
test('payment success checks the exact payee, original refund and Hong Kong time', () => {
  const before = { ...payment, state: 'draft' as const }, input = { ...financialInput('payment', before), reason: '修订' }, evidence = financialEvidence(before);
  assert.equal(financialDraftReceipt({ ...before, version: 3, current_revision_id: id(90) }, 'payment', input, evidence, before), true);
  assert.equal(financialDraftReceipt({ ...before, version: 3, current_revision_id: id(90), paid_at: '2025-02-01T02:00:00Z' }, 'payment', input, evidence, before), false);
  assert.equal(financialDraftReceipt({ ...before, version: 3, current_revision_id: id(90), employee_id: id(99) }, 'payment', input, evidence, before), false);
});
test('permission gates use actual payment.record rather than an invented create permission', () => {
  assert.equal(financialCan('payment', 'create', undefined, ['payment.record']), true);
  assert.equal(financialCan('payment', 'create', undefined, ['payment.read']), false);
  assert.equal(financialCan('cost', 'create', undefined, ['cost.read']), false);
  assert.equal(financialCan('cost', 'create', undefined, ['cost.record']), true);
  assert.equal(financialCan('cost', 'create', undefined, ['cost.create']), false);
  assert.equal(financialCan('cost', 'confirm', cost, ['cost.confirm']), false);
  assert.equal(financialCan('payment', 'allocate', payment, []), true); // Server-provided record capability includes contextual authority.
});
test('transition receipts reject stale versions, another hash/currency and missing reversal identity', () => {
  assert.equal(financialTransitionReceipt({ ...cost, version: 3 }, 'cost', cost, 'link'), true);
  assert.equal(financialTransitionReceipt(cost, 'cost', cost, 'link'), false);
  assert.equal(financialTransitionReceipt({ ...cost, version: 3, content_hash: 'd'.repeat(64) }, 'cost', cost, 'link'), false);
  assert.equal(financialTransitionReceipt({ ...cost, version: 3, state: 'reversed' }, 'cost', cost, 'reverse'), false);
  assert.equal(financialTransitionReceipt({ ...cost, version: 3, state: 'reversed', reversal_id: id(99) }, 'cost', cost, 'reverse'), true);
  assert.throws(() => financialBase({ ...cost, content_hash: '' }, '依据'));
});
test('business links only accept the correct scope and original currency with current approved payable contracts', () => {
  const source = { project_id: id(3), currency: 'HKD', approval_state: 'approved', lifecycle: 'effective', direction: 'payable' } as Schemas['Contract'];
  assert.equal(sourceEligible(source, cost, id(3)), true);
  for (const patch of [{ project_id: null }, { currency: 'USD' }, { approval_state: 'pending' }, { direction: 'receivable' }, { lifecycle: 'draft' }]) assert.equal(sourceEligible({ ...source, ...patch } as Schemas['Contract'], cost, id(3)), false);
});
test('direct settlement fixes exact cost basis and cannot allocate incoming, draft, reversal or cross-currency facts', () => {
  const body = financialAllocation(payment, cost, '部分结算', '0.3');
  assert.equal(body.cost_content_hash, cost.content_hash); assert.equal(body.cost_revision_id, cost.current_revision_id); assert.equal(body.amount, '0.3'); assert.equal(body.claim_id, null);
  for (const c of [{ ...cost, state: 'draft' as const }, { ...cost, entry_kind: 'reversal' as const }, { ...cost, currency: 'USD' }]) assert.equal(settlementEligible(c, payment), false);
  assert.throws(() => financialAllocation({ ...payment, direction: 'incoming' }, cost, '错误分配', '1'));
  assert.throws(() => financialAllocation(payment, cost, '错误分配', '0'));
});
test('employee settlement requires its own approved claim, advance line, exact supplier and linked approved revision', () => {
  const claim = { id: id(50), creator_id: id(4), approval_state: 'approved', approved_revision_id: id(51), currency: 'HKD', lines: [{ id: id(52), funding: 'employee_advance', supplier }] } as Schemas['Spend'];
  const employee = { ...payment, counterparty: null, employee_id: id(4) };
  const linked = { ...cost, links: [{ id: id(53), source_type: 'claim', source_id: claim.id, source_revision_id: claim.approved_revision_id, relation: 'reimbursed_via', title: '合成报销', reason: '固定依据', created_at: '2025-01-01T00:00:00Z' }] } as Schemas['Cost'];
  assert.equal(financialAllocation(employee, linked, '员工结算', '60', claim, id(52)).claim_line_id, id(52));
  assert.throws(() => financialAllocation(employee, linked, '错误依据', '60', { ...claim, approved_revision_id: id(59) }, id(52)));
  assert.throws(() => financialAllocation(employee, linked, '错误员工', '60', { ...claim, creator_id: id(59) }, id(52)));
  assert.throws(() => financialAllocation(employee, linked, '错误行', '60', claim, id(59)));
  assert.throws(() => financialAllocation(employee, cost, '缺少关联', '60', claim, id(52)));
});

test('expense reversal receipts retain the exact dated reverse, scope, source files and split shares', () => {
  const reverse = { ...cost, id: id(80), version: 1, entry_kind: 'reversal' as const, reverses_id: cost.id, occurred_on: '2025-02-05' };
  assert.equal(financialReversalReceipt(reverse, 'cost', cost, '2025-02-05'), true);
  assert.equal(financialReversalReceipt(reverse, 'cost', cost, '2025-02-06'), false);
  assert.equal(financialReversalReceipt({ ...reverse, allocations: [{ project_id: null, amount: '100' }] }, 'cost', cost, '2025-02-05'), false);
  assert.equal(financialReversalReceipt({ ...reverse, evidence_version_ids: [] }, 'cost', cost, '2025-02-05'), false);
});
test('cash reversal receipts check the actual UTC reverse time rather than merely a successful reversed state', () => {
  const reverse = { ...payment, id: id(81), version: 1, record_kind: 'reversal' as const, reverses_id: payment.id, paid_at: '2025-02-10T02:00:00Z' };
  assert.equal(financialReversalReceipt(reverse, 'payment', payment, '2025-02-10T10:00:00'), true);
  assert.equal(financialReversalReceipt({ ...reverse, paid_at: '2025-02-10T10:00:00Z' }, 'payment', payment, '2025-02-10T10:00:00'), false);
  assert.equal(financialReversalReceipt({ ...reverse, employee_id: id(99) }, 'payment', payment, '2025-02-10T10:00:00'), false);
  assert.equal(financialReversalReceipt({ ...reverse, record_kind: 'payment' }, 'payment', payment, '2025-02-10T10:00:00'), false);
});
