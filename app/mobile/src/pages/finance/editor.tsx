import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { financialDraftBody, financialDraftReceipt, financialEvidence, financialInput, type FinancialInput, type FinancialKind, type FinancialRecord } from '../../platform/financial-input';
import { type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { FixedFiles } from '../../platform/fixed-files';
import { Action, Empty, Fact, Notice, Section } from '../../ui';
import { ChoiceList, FileChoice, PartyChoice, SelectField, TextField, referenceEligible } from '../business/controls';

type Picking = 'project' | 'party' | 'file' | 'employee' | { allocation: number };
export function FinancialEditor({ kind, value, refund, initialProject, me, onCommitted, onCancel }: {
  kind: FinancialKind; value?: FinancialRecord; refund?: Schemas['Payment']; initialProject?: Schemas['Project']; me: Schemas['Me']; onCommitted(value: FinancialRecord): void; onCancel(): void;
}) {
  const [source] = useState(() => 'manual:' + client.operationKey());
  const initial = financialInput(kind, value, source);
  if (!value && !refund && initialProject) Object.assign(initial, { project: initialProject.id, classification: initialProject.classification });
  if (!value && refund) Object.assign(initial, { title: refund.title + ' · 退款', project: refund.project_id ?? '', classification: refund.classification, currency: refund.currency ?? '', category: refund.category, direction: 'refund' });
  if (kind === 'payment' && refund) Object.assign(initial, { fx_original_currency: refund.currency ?? '' });
  const draft = useFormDraft(me, value ? { kind: 'financial-edit', id: value.id, version: value.version, revision: value.current_revision_id, hash: value.content_hash } : { kind: 'financial-create', id: refund ? 'refund-' + refund.id : 'new-' + kind + ':' + (initialProject?.id ?? 'all'), version: 1 }, 'save', { financial: initial });
  const input = draft.fields.financial!, [evidence, setEvidence] = useState(() => value ? financialEvidence(value) : refund ? { ...financialEvidence(refund), refund: refund.id, files: [] } : financialEvidence());
  const [files, setFiles] = useState(value?.attachments ?? []), [partyName, setPartyName] = useState(value?.counterparty_evidence?.name ?? refund?.counterparty_evidence?.name ?? '');
  const [picking, setPicking] = useState<Picking>(), [error, setError] = useState<unknown>();
  const command = useCommand<FinancialRecord>(result => { draft.forget(); onCommitted(result); }, draft.binding, result => financialDraftReceipt(result, kind, input, evidence, value));
  const disabled = command.busy || command.blocked;
  function change(patch: Partial<FinancialInput>) { draft.update({ financial: { ...input, ...patch } }); setError(undefined); }
  function scope(patch: Partial<FinancialInput>) {
    change({ ...patch, allocations: [] }); setEvidence({ party: null, files: [], employee: null, refund: null }); setFiles([]); setPartyName('');
  }
  function save() {
    try { if (initialProject && input.project !== initialProject.id) throw new Error('草稿项目与当前项目不一致，请重新打开。'); const body = financialDraftBody(kind, input, evidence, value); void command.submit((kind === 'cost' ? '/costs' : '/payments') + (value ? '/' + value.id : ''), body, value ? 'PATCH' : 'POST'); }
    catch (cause) { setError(cause); }
  }
  const level = input.classification as Schemas['Cost']['classification'];
  const field = (key: keyof Omit<FinancialInput, 'allocations'>, label: string, max = 200, hint?: string, multiline = false) => <TextField name={'finance-' + key} key={key} label={label} value={input[key]} onChange={v => change({ [key]: v })} max={max} hint={hint} disabled={disabled} multiline={multiline} />;
  return <>
    <Section title={value ? '修订财务草稿' : refund ? '起草收到退款' : kind === 'cost' ? '起草实际费用' : '起草实际资金'}><View className='cp-pad'>
      <Text className='cp-paragraph'>草稿可暂缺日期、金额或凭证；正式确认／登记须补齐真实事实。保存草稿不会确认费用或发起银行付款。</Text>
      <DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
      {field('title', '财务记录标题')}
      {!value && !refund && <><Fact label='所属范围'><Text>{input.project ? '项目 ' + input.project : '公司资料'}</Text></Fact>{!initialProject && <Action secondary disabled={disabled} onClick={() => setPicking('project')}>选择所属项目</Action>}{!initialProject && input.project && <Action secondary disabled={disabled} onClick={() => scope({ project: '' })}>改为公司资料</Action>}<SelectField label='资料分类' value={input.classification} onChange={v => scope({ classification: v })} options={[[ 'internal', '内部' ], [ 'project_restricted', '项目受限' ], [ 'confidential', '机密' ]]} disabled={disabled || !!initialProject} />{field('source', '永久财务来源标识', 199, '同一经济事项或同笔资金保持同一来源标识；已保存后不可更改。')}</>}
      {value && <Fact label='固定编辑依据'><Text>版本 {value.version} · {value.current_revision_id}</Text><Text className='cp-code'>{value.content_hash}</Text></Fact>}
      {refund && <Fact label='固定退款原记录'><Text>{refund.title} · {refund.currency} {refund.amount}</Text><Text>只登记确实收到的退款并保留原收付款对象；异币实收须另填原本金和汇率凭证。</Text></Fact>}
      {kind === 'payment' && <SelectField label='资金方向' value={input.direction} onChange={v => { change({ direction: v }); if (v === 'transfer') { setEvidence(e => ({ ...e, party: null, employee: null })); setPartyName(''); } }} options={refund || value && 'refund_of_id' in value && value.refund_of_id ? [[ 'refund', '退款' ]] : [[ '', '尚未确定' ], [ 'outgoing', '支出' ], [ 'incoming', '收入' ], [ 'transfer', '内部转账' ]]} disabled={disabled || Boolean(refund || value && 'refund_of_id' in value && value.refund_of_id)} />}
      {field('date', kind === 'cost' ? '费用发生日期' : '实际发生时间（香港）', kind === 'cost' ? 10 : 19, kind === 'cost' ? 'YYYY-MM-DD；不得用审批日代替发生日。' : 'YYYY-MM-DDTHH:mm:ss；按香港本地时间填写。')}
      <TextField name='finance-currency' label='原币' value={input.currency} onChange={v => change({ currency: v.toUpperCase() })} max={3} disabled={disabled} hint='准确三位币种代码；填写实际实收币种，异币退款另核对原本金和汇率依据。' />
      {evidence.refund && input.fx_original_currency && input.fx_original_currency !== input.currency && <><Fact label='异币退款原付款币种'><Text>{input.fx_original_currency}</Text><Text>当前仅使用原付款未分配本金；费用及合同异币归属仍待后续处理。</Text></Fact>{field('fx_cost_amount', '退回原付款本金', 30)}{field('fx_rate', '实际汇率（每1原付款币兑换实收币）', 25)}{field('fx_source', '退款汇率凭证来源', 200)}</>}
      {field('amount', '实际原币金额', 30)}{field('category', '财务类别', 50)}
      {input.direction !== 'transfer' && <><Fact label={kind === 'cost' ? '固定供应方' : '实际收付款对象'}><Text>{evidence.employee ? '员工 ' + evidence.employee : partyName || (evidence.party ? '原固定往来方当前不可读' : '尚未选择')}</Text></Fact>{!refund && !(value && 'refund_of_id' in value && value.refund_of_id) && <><Action secondary disabled={disabled} onClick={() => setPicking('party')}>选择固定往来方</Action>{kind === 'payment' && input.direction === 'outgoing' && <Action secondary disabled={disabled} onClick={() => setPicking('employee')}>从批准报销选择员工</Action>}<Action secondary disabled={disabled} onClick={() => { setEvidence(e => ({ ...e, party: null, employee: null })); setPartyName(''); }}>清除收付款对象</Action></>}</>}
      {kind === 'payment' && <>{field('method', '付款方式标签', 100, '例如银行转账；不填写完整卡号、账户秘密或密钥。')}{field('external', '外部流水标识')}</>}
    </View></Section>
    {kind === 'cost' && <Section title='项目与公司分摊'><View className='cp-pad'><Text className='cp-paragraph'>确认费用时，全部项目与公司分摊合计须精确等于原额；同一范围只能出现一次。</Text>{input.allocations.map((line, index) => <View key={index} className='cp-fixed-file'><Fact label={'分摊范围 ' + (index + 1)}><Text>{line.project ? '项目 ' + line.project : '公司费用份额'}</Text></Fact><Action secondary disabled={disabled} onClick={() => setPicking({ allocation: index })}>选择分摊项目 {index + 1}</Action>{line.project && <Action secondary disabled={disabled} onClick={() => change({ allocations: input.allocations.map((v, i) => i === index ? { ...v, project: '' } : v) })}>设为公司分摊 {index + 1}</Action>}<TextField name={'allocation-' + index} label={'分摊金额 ' + (index + 1)} value={line.amount} max={30} disabled={disabled} onChange={amount => change({ allocations: input.allocations.map((v, i) => i === index ? { ...v, amount } : v) })} /><Action secondary disabled={disabled} onClick={() => { setPicking(undefined); change({ allocations: input.allocations.filter((_, i) => i !== index) }); }}>移除分摊 {index + 1}</Action></View>)}<Action secondary disabled={disabled || input.allocations.length >= 100} onClick={() => change({ allocations: [...input.allocations, { project: input.project, amount: '' }] })}>添加费用分摊</Action></View></Section>}
    <Section title='固定版本凭证'><View className='cp-pad'>{files.length > 0 && <FixedFiles files={files} />}{evidence.files.map(id => <View key={id} className='cp-fixed-file'><Text className='cp-code'>{id}</Text><Text>{files.find(f => f.version_id === id)?.name ?? '原固定凭证当前不可查阅；保存默认保留此引用。'}</Text><Action secondary disabled={disabled} onClick={() => { setEvidence(e => ({ ...e, files: e.files.filter(v => v !== id) })); setFiles(f => f.filter(v => v.version_id !== id)); }}>移除此固定凭证</Action></View>)}{!evidence.files.length && <Empty>尚未选择固定版本凭证。</Empty>}<Action secondary disabled={disabled || evidence.files.length >= 20} onClick={() => setPicking('file')}>选择财务凭证版本</Action></View></Section>
    <Section title='保存依据'><View className='cp-pad'>{field('notes', '财务备注', 20000, undefined, true)}{field('reason', '登记或修订原因', 200, undefined, true)}<Action disabled={disabled} loading={command.busy} onClick={save}>保存财务草稿</Action><Action secondary disabled={command.busy} onClick={onCancel}>返回，保留文字输入</Action></View></Section>
    {picking === 'party' && <PartyChoice project={input.project} classification={level} onClose={() => setPicking(undefined)} onSelect={party => { setEvidence(e => ({ ...e, party: { party_id: party.id, revision_id: party.current_revision_id }, employee: null })); setPartyName(party.name); setPicking(undefined); }} />}
    {picking === 'file' && <FileChoice project={input.project} classification={level} onClose={() => setPicking(undefined)} onSelect={file => { if (!evidence.files.includes(file.version_id)) { setEvidence(e => ({ ...e, files: [...e.files, file.version_id] })); setFiles(f => [...f, file]); } setPicking(undefined); }} />}
    {(picking === 'project' || typeof picking === 'object') && <ChoiceList<Schemas['Project']> title='选择当前可写项目' path='/projects?limit=20' label={v => v.name} eligible={v => !['closing', 'archived', 'cancelled'].includes(v.state) && Boolean(v.classification && referenceEligible({ classification: v.classification, project_id: null }, '', level))} onClose={() => setPicking(undefined)} onSelect={v => { if (picking === 'project') scope({ project: v.id }); else if (typeof picking === 'object') change({ allocations: input.allocations.map((line, i) => i === picking.allocation ? { ...line, project: v.id } : line) }); setPicking(undefined); }} />}
    {picking === 'employee' && <ChoiceList<Schemas['Spend']> title='选择已批准员工报销' path={'/claims?limit=20&approval_state=approved' + (initialProject ? '&project_id=' + initialProject.id : '') + (input.currency ? '&currency=' + input.currency : '')} label={v => v.title} eligible={v => v.approval_state === 'approved' && v.currency === (input.currency || null) && v.lines.some(line => line.funding === 'employee_advance')} onClose={() => setPicking(undefined)} onSelect={claim => { setEvidence(e => ({ ...e, party: null, employee: claim.creator_id })); setPartyName(''); setPicking(undefined); }} />}
  </>;
}
