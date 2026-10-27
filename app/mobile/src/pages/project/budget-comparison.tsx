import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { comparisonPath, currentBudgetComparison, currentComparisonEntries } from '@copypaste/contracts/budget-comparison';
import { useRecord } from '../../platform/collection';
import { go } from '../../platform/page';
import { exactAmount, overviewState } from '../../platform/project-overview';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Empty, Fact, Notice, Row, Section, date } from '../../ui';
import { SelectField, TextField } from '../business/controls';

export default function ProjectBudgetComparison({ projectID }: { projectID: string }) {
  const [currency, setCurrency] = useState(''), [selection, setSelection] = useState({ currency: '', revision: 0 });
  const reload = () => setSelection(old => ({ currency, revision: old.revision + 1 }));
  return <><Section title='预算与实际支出对比'><View className='cp-pad'>
    <Text className='cp-muted cp-small'>按已实施预算的完整类别与期间，核对当前可读费用的项目份额。</Text>
    <TextField name='comparison-currency' label='对比币种' value={currency} onChange={value => setCurrency(value.toUpperCase())} disabled={false} max={3} hint='填写一个已配置的三位币种代码；各币种分别查询。' />
    <Action disabled={!/^[A-Z]{3}$/.test(currency)} onClick={reload}>查询预算对比</Action>
  </View></Section>{selection.currency && <LoadedComparison key={projectID + selection.currency + selection.revision} projectID={projectID} currency={selection.currency} onReload={reload} />}</>;
}

function LoadedComparison({ projectID, currency, onReload }: { projectID: string; currency: string; onReload(): void }) {
  const query = useRecord<Schemas['BudgetComparison']>(comparisonPath(projectID, currency));
  let report: Schemas['BudgetComparison'] | undefined, error: unknown;
  try { if (query.value) report = currentBudgetComparison(query.value, projectID, currency); }
  catch { error = new MobileError('INVALID_RESPONSE', '对比资料与当前项目或币种不符，请重新读取。'); }
  return <><Notice error={query.error ?? error} />{query.loading ? <Busy /> : report && !query.error && <ComparisonDetails projectID={projectID} report={report} />}
    {Boolean(query.error ?? error) && <Action secondary onClick={onReload}>重新读取预算对比</Action>}</>;
}

function ComparisonDetails({ projectID, report }: { projectID: string; report: Schemas['BudgetComparison'] }) {
  const [view, setView] = useState<'lines' | 'sources'>('lines'), [line, setLine] = useState('');
  const money = (amount: string) => report.currency + ' ' + exactAmount(amount);
  return <>
    <Section title='当前对比依据'><View className='cp-pad'><Text className='cp-notice'>仅包含当前有权读取的费用，不代表项目全部费用或可用资金。预测、付款、未确认费用不计入实际支出。</Text><Text className='cp-muted cp-small'>读取于 {date(report.as_of)} · 项目版本 {report.project_version} · 香港发生日；预算起止日均包含。</Text></View>
      <Fact label='当前预算总额'><Text>{report.budget ? money(report.budget.total) : '尚无批准预算'}</Text>{report.budget && <Text className='cp-muted cp-small'>当前预算修订 {report.budget.revision}</Text>}</Fact>
      <Fact label='预算内可见净支出'><Text>{money(report.matched.net_amount)}</Text></Fact>
      <Fact label='预算内差额'><Text>{report.budget_variance === null ? '无预算依据' : money(report.budget_variance)}</Text></Fact>
      <Fact label='预算外可见净支出'><Text>{money(report.outside.net_amount)}</Text></Fact>
      <Fact label='全期间可见发生支出'><Text>{money(report.lifetime.net_amount)}</Text><Text className='cp-muted cp-small'>已发生 {report.lifetime.entry_count} 笔 · 未确认 {report.lifetime.unconfirmed_count} 笔</Text></Fact>
      <View className='cp-pad'><Action secondary onClick={() => setView('lines')}>查看预算明细</Action><Action secondary onClick={() => setView('sources')}>查看费用来源</Action></View>
    </Section>
    {view === 'lines' ? <Section title='预算明细对比'>{report.lines.length ? report.lines.map(item => <View key={item.index} className='cp-editor-row'>
      <Text className='cp-section-title'>第{item.index + 1}项 · {item.category}</Text><Text className='cp-muted cp-small'>{item.period_start} — {item.period_end}</Text>
      <Fact label='批准预算'><Text>{money(item.budget_amount)}</Text></Fact><Fact label='原费用／冲销份额'><Text>{money(item.expense_amount)} ／ {money(item.reversal_amount)}</Text></Fact>
      <Fact label='净发生支出'><Text>{money(item.net_amount)}</Text></Fact><Fact label='预算差额'><Text>{money(item.variance)}</Text></Fact>
      <Text className={item.over_budget ? 'cp-danger' : 'cp-muted'}>{item.over_budget ? '可见费用已超预算' : '按可见费用核对'} · {item.entry_count} 笔已发生 · {item.unconfirmed_count} 笔未确认</Text>
      <Action secondary onClick={() => { setLine(String(item.index)); setView('sources'); }}>核对第{item.index + 1}项费用</Action>
    </View>) : <Empty>{report.budget ? '当前预算已明确清空；实际费用仍保留在预算外。' : '尚无批准预算；可读费用均列入预算外，不推断预算为零。'}</Empty>}</Section>
      : <ComparisonSources key={projectID + report.basis_hash + report.as_of} projectID={projectID} report={report} initialLine={line} />}
  </>;
}

function ComparisonSources({ projectID, report, initialLine }: { projectID: string; report: Schemas['BudgetComparison']; initialLine: string }) {
  const [line, setLine] = useState(initialLine), [state, setState] = useState<'incurred' | 'draft'>('incurred'), [cursors, setCursors] = useState(['']);
  const query = useRecord<Schemas['BudgetComparisonEntries']>(comparisonPath(projectID, report.currency, { basis: report.basis_hash, line, state, cursor: cursors[cursors.length - 1], limit: 5 }));
  let page: Schemas['BudgetComparisonEntries'] | undefined, error: unknown;
  try { if (query.value) page = currentComparisonEntries(query.value, report, line, state); }
  catch { error = new MobileError('INVALID_RESPONSE', '费用明细的币种、分摊或预算依据不符，请重新查询对比。'); }
  if (query.error || error) page = undefined;
  return <Section title='费用来源核对'><View className='cp-pad'>
    <SelectField label='核对范围' value={line} disabled={query.loading} onChange={value => { setLine(value); setCursors(['']); }} options={[[ '', '全部可见费用' ], [ 'outside', '预算外费用' ], ...report.lines.map(item => [String(item.index), `第${item.index + 1}项 · ${item.category} · ${item.period_start}`] as const)]} />
    <SelectField label='费用确认状态' value={state} disabled={query.loading} onChange={value => { setState(value as typeof state); setCursors(['']); }} options={[[ 'incurred', '已发生费用' ], [ 'draft', '未确认费用' ]]} />
    <Text className='cp-muted cp-small'>第{cursors.length}页 · 每页重新核验权限与当前费用。预算或项目依据变化须重新查询对比。</Text>
  </View><Notice error={query.error ?? error} />{query.loading ? <Busy /> : page && <>
    {page.items.length ? page.items.map(entry => <View key={entry.id} className='cp-editor-row'><Row title={entry.title} subtitle={(entry.occurred_on ?? '发生日未填') + ' · ' + (entry.category || '尚未分类')} onClick={() => go('cost', entry.id)} />
      <Fact label='状态与归属'><Text>{entry.entry_kind === 'reversal' ? '冲销记录' : overviewState(entry.state)} · {entry.line_index === null ? '预算外' : `预算第${entry.line_index + 1}项`}</Text></Fact>
      <Fact label='实际项目份额'><Text>{entry.currency} {exactAmount(entry.share_amount)}</Text></Fact><Fact label='发生支出贡献'><Text>{entry.state === 'draft' ? '未计入' : entry.currency + ' ' + exactAmount(entry.signed_amount)}</Text></Fact>
    </View>) : <Empty>没有符合条件的可读费用；仅表示当前授权范围。</Empty>}
    <View className='cp-pad'>{cursors.length > 1 && <Action secondary onClick={() => setCursors(old => old.slice(0, -1))}>上一页对比费用</Action>}{page.next_cursor && <Action secondary onClick={() => setCursors(old => [...old, page!.next_cursor!])}>下一页对比费用</Action>}</View>
    <View className='cp-pad'><Text className='cp-muted cp-small'>来源读取于 {date(page.as_of)}。此对比不阻止采购，也不生成付款。</Text></View>
  </>}</Section>;
}
