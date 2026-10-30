import type { components } from './api';

type Schemas = components['schemas'];
export type BusinessCostKind = Schemas['BusinessCostSource']['type'];
export type BusinessCostBasis = Schemas['BusinessCostBasis'];
const id = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;
const decimal = /^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/;
const sourceKey = /^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$/;
const fail = (message = '业务费用依据无效，请重新从批准业务打开并核对。'): never => { throw new Error(message); };
function units(value: unknown): bigint {
  if (typeof value !== 'string' || !decimal.test(value)) return fail();
  const [whole, part = ''] = value.split('.');
  return BigInt(whole) * 1000000n + BigInt(part.padEnd(6, '0'));
}
function text(value: unknown, max: number, required = false): value is string {
  return typeof value === 'string' && [...value].length <= max && (!required || value.trim().length > 0);
}
export function businessCostKind(value: unknown): BusinessCostKind {
  return value === 'contract' || value === 'purchase' || value === 'claim' ? value : fail();
}
export function businessCostPath(kind: unknown, value: unknown) {
  if (typeof value !== 'string' || !id.test(value)) return fail();
  return '/cost-business-sources/' + businessCostKind(kind) + '/' + value;
}
export function businessCostHome(kind: BusinessCostKind, value: string, client: 'web' | 'mobile') {
  businessCostPath(kind, value);
  return client === 'web' ? '/' + ({ contract: 'contracts', purchase: 'purchases', claim: 'claims' }[kind]) + '/' + value : '/pages/business/index?kind=' + kind + '&id=' + value;
}
export function businessCostMobileEntry(kind: BusinessCostKind, value: string) {
  businessCostPath(kind, value);
  return '/pages/finance/index?kind=cost&action=create&source_type=' + kind + '&source_id=' + value;
}
export function checkedBusinessCostBasis(value: unknown, kind: BusinessCostKind, sourceID: string): BusinessCostBasis {
  businessCostPath(kind, sourceID);
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail();
  const b = value as BusinessCostBasis, s = b.source;
  if (!s || s.type !== kind || s.id !== sourceID || !Number.isSafeInteger(s.version) || s.version < 1 || !id.test(s.revision_id) || !hash.test(s.content_hash)
    || !text(b.title, 200, true) || !(b.project_id === null || typeof b.project_id === 'string' && id.test(b.project_id))
    || !['internal', 'project_restricted', 'confidential'].includes(b.classification) || b.classification === 'project_restricted' && b.project_id === null
    || !/^[A-Z]{3}$/.test(b.currency) || typeof b.as_of !== 'string' || !Number.isFinite(Date.parse(b.as_of))) return fail();
  units(b.approved_amount);
  return { source: { type: s.type, id: s.id, version: s.version, revision_id: s.revision_id, content_hash: s.content_hash }, title: b.title, project_id: b.project_id, classification: b.classification, currency: b.currency, approved_amount: b.approved_amount, as_of: b.as_of };
}
export function businessCostSeed(basis: BusinessCostBasis): Schemas['CostContent'] {
  checkedBusinessCostBasis(basis, basis.source.type, basis.source.id);
  return { title: [...('实际费用 · ' + basis.title)].slice(0, 200).join(''), currency: basis.currency, amount: null, occurred_on: null, category: '', counterparty: null, allocations: [], evidence_version_ids: [], notes: '' };
}
export function businessCostBody(basis: BusinessCostBasis, content: Schemas['CostContent'], key: string, reason: string, actual: boolean, reviewed: boolean): Schemas['BusinessCostRecord'] {
  const b = checkedBusinessCostBasis(basis, basis.source.type, basis.source.id);
  if (!actual || !reviewed) return fail('请明确核对实际发生事实及固定批准来源。');
  if (!text(reason, 200, true) || !sourceKey.test(key.trim()) || key.trim().startsWith('reversal:') || !text(content.title, 200, true)
    || !text(content.category, 50, true) || !text(content.notes, 20000)) return fail('请填写标题、费用类别、永久来源标识及登记原因，并核对文字长度。');
  if (content.currency !== b.currency || units(content.amount) <= 0n || units(content.amount) > units(b.approved_amount)) return fail('请填写与批准来源同币种的实际正数金额，且不超过批准上限。');
  const at = content.occurred_on && new Date(content.occurred_on + 'T00:00:00Z');
  if (!at || !/^\d{4}-\d{2}-\d{2}$/.test(content.occurred_on!) || !Number.isFinite(at.getTime()) || at.toISOString().slice(0, 10) !== content.occurred_on) return fail('请填写真实费用发生日期。');
  if (!content.counterparty || !id.test(content.counterparty.party_id) || !id.test(content.counterparty.revision_id) || !Array.isArray(content.evidence_version_ids) || !content.evidence_version_ids.length || content.evidence_version_ids.length > 20 || content.evidence_version_ids.some(v => !id.test(v)) || new Set(content.evidence_version_ids).size !== content.evidence_version_ids.length) return fail('请选择固定供应方修订及至少一份实际费用凭证。');
  const scopes = new Set<string | null>();
  if (!Array.isArray(content.allocations) || !content.allocations.length || content.allocations.length > 100) return fail('请填写项目／公司实际费用分摊。');
  let total = 0n;
  for (const line of content.allocations) {
    if (!(line.project_id === null || typeof line.project_id === 'string' && id.test(line.project_id)) || scopes.has(line.project_id) || units(line.amount) <= 0n) return fail('各分摊须为正数，项目／公司范围不能重复。');
    scopes.add(line.project_id); total += units(line.amount);
  }
  if (total !== units(content.amount)) return fail('全部项目与公司分摊合计须精确等于实际费用金额。');
  return { title: content.title.trim(), occurred_on: content.occurred_on, currency: content.currency, amount: content.amount,
    category: content.category.trim(), counterparty: { ...content.counterparty }, allocations: content.allocations.map(v => ({ ...v })),
    evidence_version_ids: [...content.evidence_version_ids], notes: content.notes, source: { ...b.source }, source_key: key.trim(), reason: reason.trim(), actual_incurred: true, source_reviewed: true };
}
export function businessCostReceipt(value: unknown, basis: BusinessCostBasis, body: Schemas['BusinessCostRecord']): value is Schemas['Cost'] {
  try {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
    const c = value as Schemas['Cost'];
    return id.test(c.id) && c.version === 3 && id.test(c.current_revision_id) && hash.test(c.content_hash) && c.source_key === body.source_key
      && c.state === 'confirmed' && c.entry_kind === 'expense' && c.reverses_id === null && c.reversal_id === null
      && c.project_id === basis.project_id && c.classification === basis.classification && c.currency === body.currency && units(c.amount) === units(body.amount)
      && c.title === body.title && c.occurred_on === body.occurred_on && c.category === body.category && c.notes === body.notes
      && c.counterparty?.party_id === body.counterparty?.party_id && c.counterparty?.revision_id === body.counterparty?.revision_id
      && Array.isArray(c.allocations) && c.allocations.length === body.allocations.length && new Set(c.allocations.map(v => v.project_id)).size === c.allocations.length && c.allocations.every(v => body.allocations.some(line => line.project_id === v.project_id && units(line.amount) === units(v.amount)))
      && Array.isArray(c.evidence_version_ids) && new Set(c.evidence_version_ids).size === c.evidence_version_ids.length && c.evidence_version_ids.length === body.evidence_version_ids.length && c.evidence_version_ids.every(v => body.evidence_version_ids.includes(v))
      && Array.isArray(c.links) && c.links.length === 1 && c.links[0].source_type === basis.source.type && c.links[0].source_id === basis.source.id && c.links[0].source_revision_id === basis.source.revision_id
      && c.links[0].relation === ({ contract: 'contractual_basis', purchase: 'incurred_for', claim: 'reimbursed_via' }[basis.source.type]) && c.links[0].reason === body.reason;
  } catch { return false; }
}
