import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { StaticRouter } from 'react-router-dom';
import { ApiError, type Schemas } from './api';
import { overviewItemURL, overviewListURL, projectAmount, ProjectOverviewContent } from './ProjectOverview';

type Overview = Schemas['ProjectOverview'];
type Group = Schemas['ProjectOverviewGroup'];
const item = { id: 'stable-record-id', version: 2, revision_id: 'selected-revision-id', title: 'Previously authorized contract', state: 'draft', updated_at: '2026-10-25T16:30:00Z' };
const group: Group = { kind: 'contracts', available: true, count: 8, states: [{ state: 'draft', count: 8 }], items: [item], has_more: true };
const snapshot: Overview = { project_id: 'project-a', project_version: 3, scope: 'readable_records', as_of: '2026-10-25T16:30:00Z', groups: [group], budgets: [], incurred: { available: true, unconfirmed_count: 1, totals: [{ currency: 'HKD', expense_amount: '45035996273805.050000', reversal_amount: '45035996273704.950000', net_amount: '100.100000', entry_count: 3 }] } };
function render(data = snapshot, error: unknown = null) {
  return renderToStaticMarkup(<StaticRouter location="/projects/project-a?tab=overview"><ProjectOverviewContent projectID="project-a" data={data} pending={false} error={error} refreshing={false} onRefresh={() => {}} /></StaticRouter>);
}

describe('Overview read and financial boundaries', () => {
  it('hides all cached facts and amounts when a fresh read fails', () => {
    const html = render(snapshot, new ApiError(404, 'NOT_FOUND', '当前无权读取项目概览', ''));
    expect(html).toContain('当前无权读取项目概览'); expect(html).toContain('刷新概览');
    expect(html).not.toContain(item.title); expect(html).not.toContain(item.id);
    expect(html).not.toContain('45,035,996'); expect(html).not.toContain('记账条目');
    expect(html).not.toContain('暂无可读记录');
  });
  it('never renders a snapshot for another project', () => {
    const html = render({ ...snapshot, project_id: 'project-b' });
    expect(html).not.toContain(item.title); expect(html).not.toContain('HKD');
    expect(html).toContain('概览未核实');
  });
  it('keeps restricted categories unknown and suppresses any supplied cached rows and budget amounts', () => {
    const html = render({ ...snapshot, groups: [{ ...group, kind: 'budgets', available: false, count: null }], budgets: [{ budget_id: 'hidden-budget', revision: 4, currency: 'USD', amount: '1234567.89' }], incurred: { available: false, totals: [], unconfirmed_count: null } });
    expect(html).toContain('未核实'); expect(html).not.toContain(item.title);
    expect(html).not.toContain('1234567'); expect(html).not.toContain('暂无当前批准预算');
    expect(html).not.toContain('未确认费用 0');
  });
  it('shows the readable full count independently of preview length and exact financial strings', () => {
    const html = render();
    expect(html).toContain('共 8 项可读记录'); expect(html).toContain('仅预览最近 1 项');
    expect(html).toContain('/contracts/stable-record-id');
    expect(html).toContain('45,035,996,273,805.050000'); expect(html).toContain('100.100000');
    expect(html).toContain('26/10/2026'); expect(html).toContain('00:30:00');
    expect(html).toContain('不同币种不相加');
  });
  it('uses configuration lifecycle without suggesting an observed healthy service', () => {
    const html = render({ ...snapshot, groups: [{ ...group, kind: 'services', items: [{ ...item, state: 'active' }] }] });
    expect(html).toContain('/services/stable-record-id'); expect(html).toContain('核对当前健康状态');
    expect(html).not.toContain('健康正常'); expect(html).not.toContain('/services?project_id=');
  });
});

describe('Exact amount formatting and stable navigation', () => {
  it('preserves every decimal place beyond JavaScript number precision', () => {
    expect(projectAmount('999999999999999999999999.000001')).toBe('999,999,999,999,999,999,999,999.000001');
    expect(projectAmount('-45035996273704.950000')).toBe('-45,035,996,273,704.950000');
    expect(projectAmount('0.000000')).toBe('0.000000'); expect(projectAmount('0')).toBe('0');
    expect(projectAmount('Infinity')).toBe('金额待核实'); expect(projectAmount('1e20')).toBe('金额待核实');
  });
  it('routes task and change stable IDs and preserves project list filters', () => {
    expect(overviewItemURL('p', 'tasks', 't')).toBe('/projects/p?tab=tasks&task=t');
    expect(overviewItemURL('p', 'changes', 'c')).toBe('/changes/c');
    expect(overviewItemURL('p', 'files', 'f')).toBe('/files/f');
    expect(overviewListURL('p', 'posts')).toBe('/knowledge?project_id=p');
    expect(overviewListURL('p', 'costs')).toBe('/costs?project_id=p');
    expect(overviewListURL('p', 'payments')).toBe('/payments?project_id=p');
    expect(overviewListURL('p', 'services')).toBeNull();
  });
});
