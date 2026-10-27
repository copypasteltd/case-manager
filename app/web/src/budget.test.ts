import { strict as assert } from 'node:assert';
import { test } from 'vitest';
import { budgetPolicyInput, budgetReconcileInput, tokenCount } from '@copypaste/contracts/budget';

test('budget inputs require an explicit integer and preserve a deliberate zero', () => {
  assert.equal(tokenCount('0'), 0); assert.equal(tokenCount('1000000000000'), 1_000_000_000_000);
  for (const v of ['', ' ', '-1', '1.5', '1e3', '01', '1000000000001', 'Infinity', 'NaN']) assert.throws(() => tokenCount(v));
});
test('publishing never infers a missing dimension or reason', () => {
  const fields = { organization_tokens: '100000', user_tokens: '0', project_tokens: '25000', skill_tokens: '20000' };
  assert.deepEqual(budgetPolicyInput(fields, ' reviewed allowance ', 3), { expected_version: 3, organization_tokens: 100000, user_tokens: 0, project_tokens: 25000, skill_tokens: 20000, reason: 'reviewed allowance' });
  assert.throws(() => budgetPolicyInput({ ...fields, skill_tokens: '' }, 'reason', 3)); assert.throws(() => budgetPolicyInput(fields, '', 3));
});
test('unknown usage can be reconciled to zero only with explicit counts and evidence', () => {
  assert.deepEqual(budgetReconcileInput('0', '0', 'BILL-86', 'No billable turn in the checked reference', 9), { expected_version: 9, input_tokens: 0, output_tokens: 0, evidence_reference: 'BILL-86', reason: 'No billable turn in the checked reference' });
  assert.throws(() => budgetReconcileInput('', '0', 'BILL-86', 'reason', 9)); assert.throws(() => budgetReconcileInput('0', '0', '', 'reason', 9));
});
