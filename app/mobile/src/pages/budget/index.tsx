import Taro from '@tarojs/taro';
import { Text, View } from '@tarojs/components';
import { useEffect, useState } from 'react';
import { budgetStatus } from '@copypaste/contracts/budget';
import { client } from '../../platform/taro';
import { usePage } from '../../platform/page';
import type { Page as Collection, Schemas } from '../../platform/session';
import { Action, Busy, Fact, Notice, Page, Section, date } from '../../ui';
import { SelectField, TextField } from '../business/controls';

type Entry = Schemas['BudgetEntry'];
export default function BudgetPage() {
  const [scope, setScope] = useState('mine'), [id, setID] = useState(''), [month, setMonth] = useState(''), [selection, setSelection] = useState('scope=mine');
  const [more, setMore] = useState<Collection<Entry>>(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false);
  const page = usePage(async active => ({ summary: await client.request<Schemas['BudgetSummary']>('/agent-budget?' + selection, { scope: active }), entries: await client.request<Collection<Entry>>('/agent-budget/entries?' + selection + '&limit=20', { scope: active }) }));
  useEffect(() => { setMore(undefined); void page.reload(); }, [selection, page.reload]);
  const s = page.result?.value.summary, records = more ?? page.result?.value.entries, manager = page.result?.me.actions.includes('policy.manage') ?? false;
  async function next() {
    if (!records?.next_cursor || busy || !page.scope) return; const active = page.scope; setBusy(true); setError(undefined);
    try { const next = await client.request<Collection<Entry>>('/agent-budget/entries?' + selection + '&limit=20&cursor=' + encodeURIComponent(records.next_cursor), { scope: active }); if (!active.closed) setMore({ items: [...records.items, ...next.items.filter(e => !records.items.some(old => old.id === e.id))], next_cursor: next.next_cursor }); }
    catch (cause) { if (!active.closed) setError(cause); } finally { if (!active.closed) setBusy(false); }
  }
  return <Page title='秘书用量历史' eyebrow='SECRETARY HISTORY'><Notice error={page.error ?? error} /><Section title='只读历史'><View className='cp-pad'><Text>秘书接单与执行无需配置费用额度。这里保留旧费用预留记录，不代表供应商账单。</Text></View><Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/secretary/index' })}>前往公司秘书</Action></Section>{page.loading ? <Busy /> : <>
    <Section title='历史账本查询'><View className='cp-pad'><SelectField disabled={busy} label='账本范围' value={scope} onChange={setScope} options={manager ? [['mine', '本人'], ['organization', '整个组织'], ['user', '指定员工'], ['project', '指定项目'], ['skill', '指定Skill']] : [['mine', '本人']]} />{!['mine', 'organization'].includes(scope) && <TextField disabled={busy} name='scope_id' label='范围标识' value={id} onChange={setID} max={64} />}<TextField disabled={busy} name='month' label='UTC月份（YYYY-MM）' value={month} onChange={setMonth} max={7} /><Action secondary onClick={() => setSelection(new URLSearchParams({ scope, ...(id && !['mine', 'organization'].includes(scope) ? { scope_id: id.trim() } : {}), ...(month ? { month } : {}) }).toString())}>查询历史</Action></View></Section>
    {s && <Section title={s.month + ' · 历史汇总'}><Fact label='历史运行预留'><Text>{s.reserved_tokens.toLocaleString()}</Text></Fact><Fact label='历史已记用量'><Text>{s.charged_tokens.toLocaleString()}</Text></Fact><Fact label='历史待核对'><Text>{s.pending_tokens.toLocaleString()}</Text></Fact><View className='cp-pad'><Text>历史记录保持原样，未知用量保留待核对标识。</Text>{s.unbudgeted_runs > 0 && <Text>本月另有{s.unbudgeted_runs}次运行未建立费用预留，不在此历史账本中。</Text>}</View></Section>}
    <Section title='历史运行记录'>{!records?.items.length && <View className='cp-pad'><Text>本范围暂无历史预留记录。</Text></View>}{records?.items.map(e => <View key={e.id} className='cp-pad'><Text className='cp-section-title'>{budgetStatus[e.status]}</Text><Fact label='运行'><Text selectable>{e.id}</Text></Fact><Fact label='Skill'><Text>{e.skill_id}</Text></Fact><Fact label='员工'><Text selectable>{e.actor_id}</Text></Fact><Fact label='项目'><Text selectable>{e.project_id ?? '个人会话'}</Text></Fact><Fact label='接单估算'><Text>{e.estimated_tokens.toLocaleString()}</Text></Fact><Fact label='运行预留'><Text>{e.reserved_tokens.toLocaleString()}</Text></Fact><Fact label='已记 / 待核对'><Text>{e.charged_tokens.toLocaleString()} / {e.pending_tokens.toLocaleString()}</Text></Fact><Fact label='接单时间'><Text>{date(e.created_at)}</Text></Fact>{e.evidence_reference && <Fact label='核对凭据'><Text>{e.evidence_reference} · {e.reconciliation_reason}</Text></Fact>}</View>)}{records?.next_cursor && <View className='cp-pad'><Action secondary loading={busy} onClick={() => void next()}>加载更多历史记录</Action></View>}</Section>
  </>}
    <Action secondary onClick={() => { void Taro.navigateBack().catch(() => Taro.reLaunch({ url: '/pages/settings/index' })); }}>返回上一页</Action></Page>;
}
