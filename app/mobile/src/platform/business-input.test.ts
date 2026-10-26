import test from 'node:test';
import assert from 'node:assert/strict';
import { MobileError, type Schemas } from './session';
import { MemoryDrafts } from './drafts';
import { CommandKeys } from './command-keys';
import { businessEvidence, businessInput, businessIssues, businessReceipt, copyBusinessInput, contractContent, currentBusinessPayload, formID, newBusinessLine, newBusinessPlan, requestReceipt, serverBusinessIssues, spendContent } from './business-input';
import { utf8ByteLength, utf8Bytes } from './encoding';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const file: Schemas['BusinessFileReference'] = { file_id: id(10), version_id: id(11), name: 'Fixed original', seq: 2, sha256: 'a'.repeat(64), size_bytes: '17', media_type: 'text/plain', scan_state: 'clean', scan_waiver_ref: null };
const party: Schemas['SpendSupplierEvidence'] = { party_id: id(20), revision_id: id(21), name: 'Fixed supplier', kind: 'organization' };

test('contract money and unknowns survive without floating point, with fixed party/body and exact plan identities', () => {
  const input = businessInput('contract', undefined, id(1)), evidence = businessEvidence();
  Object.assign(input, { title: ' Human contract ', reason: 'Human basis', currency: 'HKD', amount: '99999999999999.99', tax: '0.01', direction: 'payable', scheduleMode: 'scheduled' });
  input.plans = [{ ...newBusinessPlan(id(30)), title: 'First payment', amount: '99999999999999.99', due: '2028-02-29' }];
  evidence.body = file; evidence.counterparty = { id: party.party_id, revision_id: party.revision_id, name: party.name, kind: party.kind };
  assert.deepEqual(businessIssues(input), []);
  const body = contractContent(input, evidence);
  assert.equal(body.amount, '99999999999999.99'); assert.equal(body.tax_amount, '0.01');
  assert.equal(body.start_date, null); assert.equal(body.end_date, null); assert.equal(body.title, 'Human contract');
  assert.equal(body.draft_file_version_id, id(11)); assert.equal(body.counterparty_revision_id, id(21));
  assert.deepEqual(body.payment_schedule?.items[0], { id: id(30), title: 'First payment', amount: '99999999999999.99', due_date: '2028-02-29', conditions: '' });
  assert.equal('project_id' in body, false, 'Editing content cannot move the business scope');
});

test('claim row identity keeps fixed receipts, supplier and employee/company funding across reorder and deletion', () => {
  const input = businessInput('claim'), evidence = businessEvidence();
  input.lines = [{ ...newBusinessLine(id(1)), description: 'Employee cost', amount: '0.10', funding: 'employee_advance' }, { ...newBusinessLine(id(2)), description: 'Company cost', amount: '9007199254740.99', funding: 'company_paid' }];
  evidence.lineSuppliers[id(1)] = party; evidence.receipts[id(1)] = [file];
  input.lines.reverse(); const content = spendContent(input, evidence);
  assert.equal(content.lines[0].id, id(2)); assert.deepEqual(content.lines[0].receipt_version_ids, []); assert.equal(content.lines[0].declared_amount, '9007199254740.99');
  assert.equal(content.lines[1].funding, 'employee_advance'); assert.equal(content.lines[1].supplier?.revision_id, id(21)); assert.deepEqual(content.lines[1].receipt_version_ids, [id(11)]);
  input.lines = input.lines.filter(line => line.id !== id(1)); const after = spendContent(input, evidence);
  assert.equal(after.lines.length, 1); assert.deepEqual(after.lines[0].receipt_version_ids, []); assert.equal(after.lines[0].supplier, null);
});

test('100 real line inputs and a long valid request fit bounded intent reconciliation; no unresolved key is evicted', () => {
  const input = businessInput('purchase'); input.lines = Array.from({ length: 100 }, (_, index) => ({ ...newBusinessLine(id(index + 1)), description: 'A'.repeat(1000), quantity: '3.000001', price: '2.000009' }));
  const content = spendContent(input, businessEvidence()); assert.equal(content.lines.length, 100);
  const signature = JSON.stringify(content); assert.ok(signature.length > 100000); assert.ok(new TextEncoder().encode(signature).length < 1048576);
  let serial = 0; const keys = new CommandKeys(() => 'intent-' + (++serial));
  const original = keys.reserve('purchase/create/owner', signature); keys.settle('purchase/create/owner', original, 'uncertain');
  assert.equal(keys.reserve('purchase/create/owner', signature), original); assert.equal(serial, 1);
});

test('memory-only business input strips every unlisted scalar and nested evidence/secret, and defensively copies arrays', () => {
  const input = businessInput('claim'); input.lines = [newBusinessLine(id(1))]; input.plans = [newBusinessPlan(id(2))];
  const injected = { ...input, access_token: 'secret', checked: true, files: [file], supplier: party, lines: [{ ...input.lines[0], receipt_version_ids: [id(11)], password: 'secret' }], plans: [{ ...input.plans[0], mfa: '123456' }] };
  const clean = copyBusinessInput(injected)!; assert.ok(clean); assert.equal(JSON.stringify(clean).includes('secret'), false); assert.equal(JSON.stringify(clean).includes(id(11)), false);
  injected.lines[0].description = 'caller changed'; clean.lines[0].description = 'reader changed'; assert.equal(input.lines[0].description, '');
  assert.equal(copyBusinessInput({ ...input, lines: Array.from({ length: 101 }, () => newBusinessLine(id(1))) }), undefined);
  assert.equal(copyBusinessInput({ ...input, owner: 'not-an-id' }), undefined); assert.equal(copyBusinessInput({ ...input, reason: 'x'.repeat(201) }), undefined);
});

test('business inputs share the existing global text budget, exact-version authority, TTL and logout isolation', () => {
  let now = 1000; const session = { authenticated: true, epoch: 1 }, drafts = new MemoryDrafts(() => session, () => now), authority = { epoch: 1, userID: id(1), authorizationVersion: 1 };
  const input = businessInput('contract'); input.title = 'Unsubmitted title'; input.notes = 'x'.repeat(20000);
  const target = { kind: 'business-contract' as const, id: id(2), version: 1 };
  const handle = drafts.open(authority, target)!; assert.equal(handle.write({ reason: '', action: 'save', business: input }), true);
  input.notes = 'changed'; const restored = handle.read()!; assert.equal(restored.business?.notes.length, 20000);
  restored.business!.notes = 'mutated read'; assert.equal(handle.read()?.business?.notes.length, 20000);
  assert.equal(drafts.open(authority, { ...target, version: 2 })?.read(), undefined);
  for (let index = 0; index < 5; index++) drafts.open(authority, { kind: 'task', id: id(index + 100), version: 1 })!.write({ reason: 'y'.repeat(20000), action: 'start' });
  assert.equal(handle.read(), undefined, 'Nested business input must count toward the same global 100000-character budget');
  assert.equal(handle.write({ reason: '', action: 'save', business: input }), true); now += 15 * 60000; assert.equal(handle.read(), undefined);
  handle.write({ reason: '', action: 'save', business: input }); const fresh = drafts.open({ ...authority, authorizationVersion: 2 }, target)!; assert.equal(fresh.read(), undefined); assert.equal(handle.write({ reason: '', action: 'save', business: input }), false);
  session.authenticated = false; assert.equal(fresh.read(), undefined);
});

test('financial drafts preserve invalid editable input but clearly identify scientific notation, zero quantity and invalid calendar dates', () => {
  const input = businessInput('purchase'); input.title = 'Draft'; input.reason = 'Human input';
  input.lines = [{ ...newBusinessLine(id(1)), quantity: '0.000000', price: '1e6', date: '2026-02-29' }];
  const paths = businessIssues(input).map(issue => issue.path);
  assert.ok(paths.includes('line-' + id(1) + '-quantity')); assert.ok(paths.includes('line-' + id(1) + '-price')); assert.ok(paths.includes('line-' + id(1) + '-date'));
  assert.equal(copyBusinessInput(input)?.lines[0].price, '1e6', 'A local invalid field stays editable instead of being rounded or silently discarded');
  input.lines[0].quantity = '0.000001'; input.lines[0].price = '1.123456'; input.lines[0].date = '2028-02-29'; assert.deepEqual(businessIssues(input), []);
});

test('missing current evidence stops editing and factual display retains the authoritative original reference IDs', () => {
  const value = { ...spendContent(businessInput('claim'), businessEvidence()), id: id(1), kind: 'claim', current_revision_id: id(2), content_hash: 'b'.repeat(64), project_id: null, classification: 'internal', document_no: 'C-1', payload_schema_version: 1,
    amount: null, employee_reimbursement_amount: null, line_values: [], suppliers: [], attachments: [], supplier_candidates: [{ party_id: id(20), revision_id: id(21) }], quote_version_ids: [id(11)] } as unknown as Schemas['Spend'];
  assert.equal(businessEvidence(value).unresolved, true);
  const payload = currentBusinessPayload('claim', value); assert.equal(payload.kind, 'claim');
  if (payload.kind === 'claim') { assert.deepEqual(payload.content.quote_version_ids, [id(11)]); assert.deepEqual(payload.content.supplier_candidates, value.supplier_candidates); }
});

test('server field errors map to real fields or an explicit summary location; decimal input is never reinterpreted', () => {
  const input = businessInput('contract'); input.amount = '0.000001';
  const error = new MobileError('REQUIRED_BUSINESS_FIELD', 'Review fields', 422, 'request-1', [{ field: 'currency', safe_message: 'Unsupported currency' }, { field: 'unknown_field', safe_message: 'Review content' }]);
  assert.deepEqual(serverBusinessIssues(error, input), [{ path: 'currency', label: '币种', message: 'Unsupported currency' }, { path: 'save', label: '保存资料', message: 'Review content' }]);
  assert.equal(input.amount, '0.000001'); assert.deepEqual(serverBusinessIssues(new MobileError('FORBIDDEN', 'No permission', 403), input), []);
});

test('new line/plan IDs derive from public independent intent values and remain stable within a single intent', () => {
  const first = formID('mobile:wechat:public-server-nonce:1'), second = formID('mobile:wechat:public-server-nonce:2');
  assert.match(first, /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/);
  assert.equal(first, formID('mobile:wechat:public-server-nonce:1')); assert.notEqual(first, second);
  const encoder = globalThis.TextEncoder;
  try { Reflect.deleteProperty(globalThis, 'TextEncoder'); assert.equal(first, formID('mobile:wechat:public-server-nonce:1')); }
  finally { Reflect.set(globalThis, 'TextEncoder', encoder); }
});

test('UTF-8 limits count actual wire bytes and work without browser encoding globals', () => {
  const values = ['', 'plain ASCII', '合同金额 12.34', '😀\u0000\ud800\udc00\ud800tail\udc00', JSON.stringify({ reason: '香港😀', invalidSurrogate: '\ud800' })];
  const expected = values.map(value => new TextEncoder().encode(value)); const encoder = globalThis.TextEncoder;
  try {
    Reflect.deleteProperty(globalThis, 'TextEncoder');
    values.forEach((value, index) => { assert.deepEqual(utf8Bytes(value), expected[index]); assert.equal(utf8ByteLength(value), expected[index].length); });
    const exact = '中'.repeat(349525) + 'a'; assert.equal(utf8ByteLength(exact), 1048576); assert.equal(utf8ByteLength(exact + 'a'), 1048577);
  } finally { Reflect.set(globalThis, 'TextEncoder', encoder); }
});

test('business and request receipts must match actual identity, new version and fixed request basis before input can be cleared', () => {
  const business = { id: id(1), version: 2, current_revision_id: id(2), content_hash: 'a'.repeat(64), can_edit: true, contract_no: 'C-1' };
  assert.equal(businessReceipt(business, 'contract', id(1), 2), true);
  assert.equal(businessReceipt(business, 'contract', id(1), 3), false); assert.equal(businessReceipt(business, 'contract', id(3)), false); assert.equal(businessReceipt(business, 'purchase'), false);
  const receipt = { id: id(3), version: 4, kind: 'claim', business_id: id(1), current_revision_id: id(4), content_hash: 'b'.repeat(64), state: 'in_review', can_submit: false, can_revise: false, can_withdraw: true,
    payload: { kind: 'claim', business_id: id(1), business_revision_id: id(2) } };
  const expected = { kind: 'claim', businessID: id(1), id: id(3), minimumVersion: 4, revisionID: id(4), businessRevisionID: id(2), state: 'in_review' as const };
  assert.equal(requestReceipt(receipt, expected), true);
  assert.equal(requestReceipt(receipt, { ...expected, revisionID: undefined, previousRevisionID: id(5) }), true);
  assert.equal(requestReceipt(receipt, { ...expected, revisionID: undefined, previousRevisionID: receipt.current_revision_id }), false, 'Submitting fixes a new request revision; an unchanged draft revision is not a valid submit receipt');
  for (const change of [{ id: id(5) }, { version: 3 }, { state: 'draft' }, { current_revision_id: id(5) }, { content_hash: 'bad' }, { payload: null }, { payload: { ...receipt.payload, business_revision_id: id(5) } }, { payload: { ...receipt.payload, kind: 'contract' } }]) assert.equal(requestReceipt({ ...receipt, ...change }, expected), false);
});
