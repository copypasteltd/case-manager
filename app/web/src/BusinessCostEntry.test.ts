import { describe, expect, it } from 'vitest';
import { businessCostBody, businessCostHome, businessCostMobileEntry, businessCostPath, businessCostReceipt, businessCostSeed, checkedBusinessCostBasis, type BusinessCostBasis } from '@copypaste/contracts/cost-business';
const sourceID = '713cfcdf-79a5-4a4e-b9df-e8ac234cffca', projectID = '813cfcdf-79a5-4a4e-b9df-e8ac234cffca', revision = '913cfcdf-79a5-4a4e-b9df-e8ac234cffca', costID = 'a13cfcdf-79a5-4a4e-b9df-e8ac234cffca';
const basis = (): BusinessCostBasis => ({ source: { type: 'purchase', id: sourceID, version: 4, revision_id: revision, content_hash: 'a'.repeat(64) }, project_id: projectID, classification: 'project_restricted', currency: 'HKD', approved_amount: '100.000000', title: 'Approved service', as_of: '2026-10-30T00:00:00Z' });
const content = () => ({ ...businessCostSeed(basis()), title: 'Actual service', occurred_on: '2025-02-10', amount: '25.01', category: 'service', counterparty: { party_id: sourceID, revision_id: revision }, allocations: [{ project_id: projectID, amount: '20' }, { project_id: null, amount: '5.01' }], evidence_version_ids: [revision, sourceID] });
const body = () => businessCostBody(basis(), content(), 'manual:fixed-economic-source', 'Human reviewed actual facts', true, true);
const receipt = () => ({ ...body(), id: costID, version: 3, current_revision_id: revision, content_hash: 'b'.repeat(64), state: 'confirmed', entry_kind: 'expense', reverses_id: null, reversal_id: null, project_id: projectID, classification: 'project_restricted', links: [{ source_type: 'purchase', source_id: sourceID, source_revision_id: revision, relation: 'incurred_for', reason: body().reason }] });
describe('Actual business cost recording', () => {
  it('seeds only editable descriptive context while leaving occurred facts unknown', () => {
    for (const type of ['purchase', 'claim', 'contract'] as const) { const b = basis(); b.source.type = type; const c = businessCostSeed(b); expect(c.currency).toBe('HKD'); expect(c.amount).toBeNull(); expect(c.occurred_on).toBeNull(); expect(c.counterparty).toBeNull(); expect(c.evidence_version_ids).toEqual([]); expect(c.allocations).toEqual([]); }
  });
  it('rejects foreign, malformed or unsafe source previews and copies the exact approved basis', () => {
    const b = basis(); const checked = checkedBusinessCostBasis(b, 'purchase', sourceID); b.source.version = 9; expect(checked.source.version).toBe(4);
    for (const v of [{ ...basis(), source: { ...basis().source, id: projectID } }, { ...basis(), source: { ...basis().source, version: Number.MAX_SAFE_INTEGER + 1 } }, { ...basis(), project_id: null }, { ...basis(), as_of: 'unknown' }]) expect(() => checkedBusinessCostBasis(v, 'purchase', sourceID)).toThrow();
  });
  it('preserves precise monetary strings, fixed evidence and separate company shares', () => {
    const c = content(), b = businessCostBody(basis(), c, 'manual:source', 'Actual facts', true, true); c.allocations[0].amount = '99'; c.evidence_version_ids.length = 0; expect(b.allocations[0].amount).toBe('20'); expect(b.evidence_version_ids).toHaveLength(2); expect(b.actual_incurred && b.source_reviewed).toBe(true); expect('classification' in b || 'project_id' in b).toBe(false);
    const big = basis(); big.approved_amount = '45035996273704.95'; const v = { ...content(), amount: big.approved_amount, allocations: [{ project_id: projectID, amount: big.approved_amount }] }; expect(businessCostBody(big, v, 'manual:large-exact', 'Reviewed', true, true).amount).toBe(big.approved_amount);
  });
  it('requires actual facts and fresh acknowledgements rather than inferred approval amounts', () => {
    for (const [actual, reviewed] of [[false, true], [true, false]]) expect(() => businessCostBody(basis(), content(), 'manual:source', 'Reviewed', actual, reviewed)).toThrow();
    for (const patch of [{ amount: '101' }, { currency: 'USD' }, { occurred_on: '2025-02-29' }, { amount: '0' }, { evidence_version_ids: [] }, { allocations: [{ project_id: projectID, amount: '25' }] }]) expect(() => businessCostBody(basis(), { ...content(), ...patch }, 'manual:source', 'Reviewed', true, true)).toThrow();
  });
  it('accepts only the complete confirmed expense and exact ordinary business association', () => {
    expect(businessCostReceipt(receipt(), basis(), body())).toBe(true);
    for (const patch of [{ version: 1 }, { state: 'draft' }, { project_id: null }, { links: [] }, { amount: '25' }, { evidence_version_ids: [revision, revision] }, { allocations: [{ project_id: projectID, amount: '20' }, { project_id: projectID, amount: '20' }] }]) expect(businessCostReceipt({ ...receipt(), ...patch }, basis(), body())).toBe(false);
    const wrong = receipt(); wrong.links[0].source_revision_id = costID; expect(businessCostReceipt(wrong, basis(), body())).toBe(false);
  });
  it('uses fixed type and identifier routes without a client-supplied money or project context', () => {
    expect(businessCostPath('claim', sourceID)).toBe('/cost-business-sources/claim/' + sourceID); expect(businessCostHome('contract', sourceID, 'web')).toBe('/contracts/' + sourceID); expect(businessCostMobileEntry('purchase', sourceID)).toBe('/pages/finance/index?kind=cost&action=create&source_type=purchase&source_id=' + sourceID);
    for (const kind of ['payment', '../contracts', 'contract?project_id=x']) expect(() => businessCostPath(kind, sourceID)).toThrow(); expect(() => businessCostPath('claim', sourceID + '?amount=1')).toThrow();
  });
});
