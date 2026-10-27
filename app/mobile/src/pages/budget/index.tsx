import Taro from '@tarojs/taro';
import { Text, View } from '@tarojs/components';
import { useEffect, useState } from 'react';
import { budgetLabels, budgetPolicyInput, budgetReconcileInput, budgetStatus } from '@copypaste/contracts/budget';
import { client } from '../../platform/taro';
import { usePage } from '../../platform/page';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import type { Page as Collection, Schemas } from '../../platform/session';
import { Action, Busy, Fact, Notice, Page, Section, date } from '../../ui';
import { SelectField, TextField } from '../business/controls';

type Entry = Schemas['BudgetEntry'];
export default function BudgetPage() {
  const [scope, setScope] = useState('mine'), [id, setID] = useState(''), [month, setMonth] = useState(''), [selection, setSelection] = useState('scope=mine');
  const [editing, setEditing] = useState(false), [entry, setEntry] = useState<Entry>(), [more, setMore] = useState<Collection<Entry>>(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false);
  const page = usePage(async active => ({ summary: await client.request<Schemas['BudgetSummary']>('/agent-budget?' + selection, { scope: active }), entries: await client.request<Collection<Entry>>('/agent-budget/entries?' + selection + '&limit=20', { scope: active }) }));
  useEffect(() => { setMore(undefined); void page.reload(); }, [selection, page.reload]);
  const s = page.result?.value.summary, records = more ?? page.result?.value.entries, manager = page.result?.me.actions.includes('policy.manage') ?? false;
  async function changed() { setEntry(undefined); setEditing(false); setMore(undefined); await page.reload(); }
  async function next() {
    if (!records?.next_cursor || busy || !page.scope) return; const active = page.scope; setBusy(true); setError(undefined);
    try { const next = await client.request<Collection<Entry>>('/agent-budget/entries?' + selection + '&limit=20&cursor=' + encodeURIComponent(records.next_cursor), { scope: active }); if (!active.closed) setMore({ items: [...records.items, ...next.items.filter(e => !records.items.some(old => old.id === e.id))], next_cursor: next.next_cursor }); }
    catch (cause) { if (!active.closed) setError(cause); } finally { if (!active.closed) setBusy(false); }
  }
  return <Page title='秘书额度与用量' eyebrow='TOKEN ACCOUNT'><Notice error={page.error ?? error} />{page.loading ? <Busy /> : <>
    {editing || entry ? <BudgetEditor key={entry?.id ?? ('policy-' + (s?.policy?.version ?? 0))} policy={s?.policy ?? null} entry={entry} onSaved={() => void changed()} onClose={() => { setEditing(false); setEntry(undefined); }} /> : <>
      <Section title='查询Token账本'><View className='cp-pad'><SelectField label='账本范围' value={scope} options={[['mine', '本人'] as const, ...(manager ? [['organization', '整个组织'] as const, ['user', '指定员工'] as const, ['project', '指定项目'] as const, ['skill', '指定Skill'] as const] : [])]} disabled={busy} onChange={setScope} />
        {!['mine', 'organization'].includes(scope) && <TextField name='scope_id' label='范围标识' value={id} onChange={setID} disabled={busy} max={64} />}<TextField name='month' label='UTC月份' value={month} onChange={setMonth} disabled={busy} max={7} hint='YYYY-MM，留空查本月。' />
        <Action secondary onClick={() => { setBusy(false); setSelection(new URLSearchParams({ scope, ...(month ? { month } : {}), ...(!['mine', 'organization'].includes(scope) ? { scope_id: id.trim() } : {}) }).toString()); }}>查询账本</Action>{manager && <Action onClick={() => setEditing(true)}>配置月度额度</Action>}</View></Section>
      {s && <Section title={s.month + ' · 选定范围'}><Fact label='当前策略额度'><Text>{s.limit_tokens?.toLocaleString() ?? '未配置'}</Text></Fact><Fact label='运行预留'><Text>{s.reserved_tokens.toLocaleString()}</Text></Fact><Fact label='已记用量（含待核对）'><Text>{s.charged_tokens.toLocaleString()}</Text></Fact><Fact label='其中待核对'><Text>{s.pending_tokens.toLocaleString()}</Text></Fact><Fact label='可用额度'><Text>{s.available_tokens?.toLocaleString() ?? '未配置'}</Text></Fact><View className='cp-pad'><Text>输入Token包含缓存，输出包含推理，均只计一次。已启动但用量未知的运行按预留量计入待核对；取消不会假定免费。这不是供应商金额账单或金额硬上限。</Text>{s.unbudgeted_runs > 0 && <Text>本月另有{s.unbudgeted_runs}次升级前运行未纳入账本。</Text>}</View></Section>}
      <Section title='运行账本'>{!records?.items.length && <View className='cp-pad'><Text>本范围暂无预留记录。</Text></View>}{records?.items.map(e => <View key={e.id} className='cp-pad'><Text className='cp-section-title'>{budgetStatus[e.status]}</Text><Fact label='运行'><Text selectable>{e.id}</Text></Fact><Fact label='Skill'><Text>{e.skill_id}</Text></Fact><Fact label='员工'><Text selectable>{e.actor_id}</Text></Fact><Fact label='项目'><Text selectable>{e.project_id ?? '个人会话'}</Text></Fact><Fact label='接单估算'><Text>{e.estimated_tokens.toLocaleString()}</Text></Fact><Fact label='运行预留'><Text>{e.reserved_tokens.toLocaleString()}</Text></Fact><Fact label='已记 / 待核对'><Text>{e.charged_tokens.toLocaleString()} / {e.pending_tokens.toLocaleString()}</Text></Fact><Fact label='接单时间'><Text>{date(e.created_at)}</Text></Fact>{e.evidence_reference && <Fact label='核对凭据'><Text>{e.evidence_reference} · {e.reconciliation_reason}</Text></Fact>}{manager && e.status === 'pending' && <Action secondary onClick={() => setEntry(e)}>核对用量</Action>}</View>)}{records?.next_cursor && <View className='cp-pad'><Action secondary loading={busy} onClick={() => void next()}>加载更多记录</Action></View>}</Section>
    </>}</>}
    <Action secondary onClick={() => { void Taro.navigateBack().catch(() => Taro.reLaunch({ url: '/pages/settings/index' })); }}>返回上一页</Action></Page>;
}
function BudgetEditor({ policy, entry, onSaved, onClose }: { policy: Schemas['BudgetPolicy'] | null; entry?: Entry; onSaved(): void; onClose(): void }) {
  const [values, setValues] = useState<Record<keyof typeof budgetLabels, string>>({ organization_tokens: policy ? String(policy.organization_tokens) : '', user_tokens: policy ? String(policy.user_tokens) : '', project_tokens: policy ? String(policy.project_tokens) : '', skill_tokens: policy ? String(policy.skill_tokens) : '' });
  const [input, setInput] = useState(''), [output, setOutput] = useState(''), [evidence, setEvidence] = useState(''), [reason, setReason] = useState(''), [error, setError] = useState<unknown>(), [fresh, setFresh] = useState(false);
  const command = useCommand(onSaved, entry ? 'budget-reconcile:' + entry.id : 'budget-policy:' + (policy?.version ?? 0));
  async function submit() { setError(undefined); try { if (entry) await command.submit('/agent-budget/entries/' + entry.id + '/reconcile', budgetReconcileInput(input, output, evidence, reason, entry.run_version)); else await command.submit('/agent-budget/policies', budgetPolicyInput(values, reason, policy?.version ?? 0)); } catch (cause) { setError(cause); } }
  const disabled = command.busy || command.blocked;
  return <Section title={entry ? '凭证核对运行用量' : '配置秘书月度Token额度'}><View className='cp-pad'><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Text>{entry ? '仅核对已终止且用量未知的运行。有凭据确认零用量时可明确填0。' : '所有入口共享组织额度；员工、项目、Skill分别累计。0表示停止相应范围接新运行，已接单预留保留。'}</Text>
    {entry ? <><TextField name='input_tokens' label='实际输入Token' value={input} onChange={setInput} disabled={disabled} /><TextField name='output_tokens' label='实际输出Token' value={output} onChange={setOutput} disabled={disabled} /><TextField name='evidence_reference' label='核对凭据编号' value={evidence} onChange={setEvidence} disabled={disabled} max={120} /></> : (Object.keys(budgetLabels) as (keyof typeof budgetLabels)[]).map(k => <TextField key={k} name={k} label={budgetLabels[k]} value={values[k]} onChange={value => setValues(old => ({ ...old, [k]: value }))} disabled={disabled} />)}
    <TextField name='reason' label={entry ? '核对原因' : '发布原因'} value={reason} onChange={setReason} disabled={disabled} max={120} /><MFAConfirmation disabled={disabled} onFresh={setFresh} />
    <Action disabled={!fresh || disabled} loading={command.busy} onClick={() => void submit()}>确认{entry ? '核对' : '发布'}</Action><Action secondary disabled={command.busy} onClick={onClose}>返回账本</Action>
  </View></Section>;
}
