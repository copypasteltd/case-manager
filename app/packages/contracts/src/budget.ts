import type { components } from './api';
type Schemas = components['schemas'];
export const budgetLabels = { organization_tokens: '组织月度额度', user_tokens: '每位员工月度额度', project_tokens: '每个项目月度额度', skill_tokens: '每项Skill月度额度' } as const;
export const budgetStatus = { reserved: '已预留', settled: '按已知用量结算', pending: '用量待核对', released: '未启动已释放', reconciled: '已人工核对' } as const;
export function tokenCount(value: string): number {
  if (!/^(0|[1-9]\d{0,12})$/.test(value) || Number(value) > 1_000_000_000_000) throw new Error('Token数量须为0到1000000000000的整数；不能留空。');
  return Number(value);
}
function required(value: string, label: string): string {
  const s = value.trim(); if (!s || [...s].length > 120) throw new Error(label + '须为1至120个字符。'); return s;
}
export function budgetPolicyInput(fields: Record<keyof typeof budgetLabels, string>, reason: string, version: number): Schemas['BudgetPolicyInput'] {
  return { expected_version: version, organization_tokens: tokenCount(fields.organization_tokens), user_tokens: tokenCount(fields.user_tokens),
    project_tokens: tokenCount(fields.project_tokens), skill_tokens: tokenCount(fields.skill_tokens), reason: required(reason, '发布原因') };
}
export function budgetReconcileInput(input: string, output: string, evidence: string, reason: string, version: number): Schemas['BudgetReconcileInput'] {
  return { expected_version: version, input_tokens: tokenCount(input), output_tokens: tokenCount(output), evidence_reference: required(evidence, '核对凭据编号'), reason: required(reason, '核对原因') };
}
