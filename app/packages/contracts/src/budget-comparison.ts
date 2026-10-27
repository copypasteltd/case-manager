import type { components } from './api';

type Schemas = components['schemas'];
const currencyCode = /^[A-Z]{3}$/, hash = /^[0-9a-f]{64}$/;
const positiveText = /^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$/, signedText = /^-?(0|[1-9][0-9]*)(\.[0-9]{1,6})?$/;

export function comparisonPath(projectID: string, currency: string, detail?: {
  basis: string; line?: string; state?: 'incurred' | 'draft'; limit?: number; cursor?: string;
}) {
  if (!/^[0-9a-f-]{36}$/i.test(projectID) || !currencyCode.test(currency)) throw new Error('请选择项目并明确填写三位币种代码。');
  const query = new URLSearchParams({ currency });
  if (detail) {
    if (!hash.test(detail.basis)) throw new Error('请先读取当前预算对比依据。');
    const line = detail.line ?? '', limit = detail.limit ?? 10;
    if (line && !/^(outside|[0-9]|[1-9][0-9])$/.test(line)) throw new Error('请重新选择预算明细。');
    if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new Error('分页数量无效。');
    query.set('basis_hash', detail.basis); query.set('state', detail.state ?? 'incurred'); query.set('limit', String(limit));
    if (line) query.set('line', line);
    if (detail.cursor) query.set('cursor', detail.cursor);
  }
  return '/projects/' + projectID + '/budget-comparison' + (detail ? '/entries' : '') + '?' + query;
}

export function currentBudgetComparison(value: Schemas['BudgetComparison'], projectID: string, currency: string) {
  if (!value || value.project_id !== projectID || value.currency !== currency || !hash.test(value.basis_hash)
    || !Number.isSafeInteger(value.project_version) || value.project_version < 1 || value.scope !== 'authorized_records_only'
    || value.timezone !== 'Asia/Hong_Kong' || !Array.isArray(value.lines) || value.lines.length > 100
    || (value.budget === null && (value.lines.length !== 0 || value.budget_variance !== null))) {
    throw new Error('对比资料与当前项目、币种或预算依据不符，请重新读取。');
  }
  for (const [index, line] of value.lines.entries()) {
    if (line.index !== index || !positiveText.test(line.budget_amount) || !signedText.test(line.net_amount) || !signedText.test(line.variance)) {
      throw new Error('预算明细或金额尚未核实，请重新读取。');
    }
  }
  return value;
}

export function currentComparisonEntries(value: Schemas['BudgetComparisonEntries'], report: Schemas['BudgetComparison'], line: string, state: 'incurred' | 'draft') {
  if (!value || value.basis_hash !== report.basis_hash || !Array.isArray(value.items)) throw new Error('费用明细的预算依据已变化，请重新查询对比。');
  for (const entry of value.items) {
    if (entry.currency !== report.currency || !positiveText.test(entry.share_amount) || !signedText.test(entry.signed_amount)
      || (entry.line_index !== null && (!Number.isInteger(entry.line_index) || entry.line_index < 0 || entry.line_index >= report.lines.length))
      || (line === 'outside' && entry.line_index !== null) || (line && line !== 'outside' && entry.line_index !== Number(line))
      || (state === 'draft' ? entry.state !== 'draft' || !/^0(\.0{1,6})?$/.test(entry.signed_amount) : entry.state === 'draft')
      || (entry.entry_kind === 'expense' && entry.signed_amount.startsWith('-'))
      || (entry.entry_kind === 'reversal' && state !== 'draft' && !entry.signed_amount.startsWith('-'))) {
      throw new Error('费用明细的币种、分摊或确认状态不符，请重新读取。');
    }
  }
  return value;
}
