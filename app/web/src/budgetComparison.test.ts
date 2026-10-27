import { test, expect } from 'vitest';
import { comparisonPath, currentBudgetComparison, currentComparisonEntries } from '@copypaste/contracts/budget-comparison';
import type { Schemas } from './api';

const pid = '11111111-1111-4111-8111-111111111111', basis = 'a'.repeat(64);
const report = { project_id: pid, project_version: 2, currency: 'HKD', basis_hash: basis, scope: 'authorized_records_only', timezone: 'Asia/Hong_Kong', budget: null, budget_variance: null, lines: [] } as unknown as Schemas['BudgetComparison'];

test('a cached report for a different project, currency or authority scope is rejected', () => {
  expect(currentBudgetComparison(report, pid, 'HKD')).toBe(report);
  for (const change of [{ project_id: '22222222-2222-4222-8222-222222222222' }, { currency: 'USD' }, { scope: 'all' }]) {
    expect(() => currentBudgetComparison({ ...report, ...change } as Schemas['BudgetComparison'], pid, 'HKD')).toThrow();
  }
});
test('absent budget never becomes an implied zero plan or variance', () => {
  expect(() => currentBudgetComparison({ ...report, budget_variance: '0' }, pid, 'HKD')).toThrow();
});
test('source navigation binds the exact basis, currency, filter and opaque cursor', () => {
  const path = comparisonPath(pid, 'HKD', { basis, line: 'outside', state: 'draft', cursor: 'signed+/=', limit: 5 });
  const params = new URL('https://local.invalid' + path).searchParams;
  expect(params.get('cursor')).toBe('signed+/='); expect(params.get('basis_hash')).toBe(basis); expect(params.get('line')).toBe('outside');
  for (const line of ['-1', '00', '100']) expect(() => comparisonPath(pid, 'HKD', { basis, line })).toThrow();
});
test('details from an old budget basis cannot be appended after a budget change', () => {
  expect(() => currentComparisonEntries({ basis_hash: 'b'.repeat(64), items: [], next_cursor: null, as_of: '' }, report, '', 'incurred')).toThrow();
});
test('large exact decimal shares and negative reversal contributions stay strings', () => {
  const entry = { currency: 'HKD', share_amount: '99999999999999.123456', signed_amount: '-99999999999999.123456', entry_kind: 'reversal', state: 'confirmed', line_index: null } as Schemas['BudgetComparisonEntry'];
  const page = { basis_hash: basis, items: [entry], next_cursor: null, as_of: '' };
  expect(currentComparisonEntries(page, report, 'outside', 'incurred').items[0].signed_amount).toBe('-99999999999999.123456');
  expect(() => currentComparisonEntries({ ...page, items: [{ ...entry, currency: 'USD' }] }, report, '', 'incurred')).toThrow();
});
test('draft sources cannot claim an incurred contribution or enter confirmed detail pages', () => {
  const entry = { currency: 'HKD', share_amount: '25.000000', signed_amount: '0', entry_kind: 'expense', state: 'draft', line_index: null } as Schemas['BudgetComparisonEntry'];
  const page = { basis_hash: basis, items: [entry], next_cursor: null, as_of: '' };
  expect(currentComparisonEntries(page, report, '', 'draft').items).toHaveLength(1);
  expect(() => currentComparisonEntries(page, report, '', 'incurred')).toThrow();
  expect(() => currentComparisonEntries({ ...page, items: [{ ...entry, signed_amount: '25' }] }, report, '', 'draft')).toThrow();
});
