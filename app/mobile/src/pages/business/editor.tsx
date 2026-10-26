import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { type Schemas } from '../../platform/session';
import { useFormDraft } from '../../platform/form';
import { useRecord } from '../../platform/collection';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { businessEvidence, businessInput, businessIssues, businessNames, businessReceipt, businessRoutes, contractContent, formID, newBusinessLine, newBusinessPlan, serverBusinessIssues, spendContent, type Business, type BusinessInput, type BusinessKind, type BusinessLine, type BusinessPlan, type InputIssue } from '../../platform/business-input';
import { Action, Empty, Fact, Notice, Section, State } from '../../ui';
import { semantic } from '../../platform/components';
import { ChoiceList, FileChoice, PartyChoice, SelectField, TextField } from './controls';

type Picking = 'project' | 'owner' | 'parent' | 'counterparty' | 'body' | 'candidate' | 'quote' | { supplier: string } | { receipt: string };
export function BusinessEditor({ kind, value, me, onCommitted, onCancel }: { kind: BusinessKind; value?: Business; me: Schemas['Me']; onCommitted(value: Business): void; onCancel(): void }) {
  const draft = useFormDraft(me, { kind: ('business-' + kind) as 'business-contract' | 'business-purchase' | 'business-claim', id: value?.id ?? 'new-' + kind, version: value?.version ?? 1 }, 'save', { business: businessInput(kind, value, me.user.id) });
  const input = draft.fields.business!, [evidence, setEvidence] = useState(() => businessEvidence(value));
  const project = useRecord<Schemas['Project']>(input.project && me.actions.includes('project.read') ? '/projects/' + input.project : null);
  const parent = useRecord<Schemas['Contract']>(input.parent ? '/contracts/' + input.parent : null);
  const [picking, setPicking] = useState<Picking>(), [lineID, setLineID] = useState<string | undefined>(input.lines[0]?.id), [planID, setPlanID] = useState<string | undefined>(input.plans[0]?.id);
  const [issues, setIssues] = useState<InputIssue[]>([]), [focus, setFocus] = useState<string>(), [validation, setValidation] = useState(false);
  const focusTimer = useRef<ReturnType<typeof setTimeout>>();
  useEffect(() => () => { if (focusTimer.current) clearTimeout(focusTimer.current); }, []);
  const command = useCommand<Business>(result => { draft.forget(); onCommitted(result); }, draft.binding, result => businessReceipt(result, kind, value?.id, value ? value.version + 1 : 1));
  useEffect(() => {
    const serverIssues = serverBusinessIssues(command.error, input);
    if (serverIssues.length) { setIssues(serverIssues); setValidation(true); locate(serverIssues[0]); }
  }, [command.error]);
  const disabled = command.busy || command.blocked;
  function change(patch: Partial<BusinessInput>) { draft.update({ business: { ...input, ...patch } }); setFocus(undefined); }
  function changeLine(id: string, patch: Partial<BusinessLine>) { change({ lines: input.lines.map(line => line.id === id ? { ...line, ...patch } : line) }); }
  function changePlan(id: string, patch: Partial<BusinessPlan>) { change({ plans: input.plans.map(plan => plan.id === id ? { ...plan, ...patch } : plan) }); }
  function scope(patch: Partial<BusinessInput>) { change(patch); setEvidence(businessEvidence()); }
  const level = input.classification as Schemas['Contract']['classification'];
  function locate(issue: InputIssue) {
    if (issue.path.startsWith('line-')) setLineID(issue.path.slice(5, 41));
    if (issue.path.startsWith('plan-')) setPlanID(issue.path.slice(5, 41));
    setFocus(issue.path);
    if (focusTimer.current) clearTimeout(focusTimer.current);
    focusTimer.current = setTimeout(() => { void Taro.pageScrollTo({ selector: '#oa-' + issue.path, duration: 150 }).catch(() => {}); }, 30);
  }
  function save() {
    if (disabled) return;
    const errors = businessIssues(input); setIssues(errors); setValidation(true);
    if (errors.length) { locate(errors[0]); return; }
    if (evidence.unresolved) return;
    const reason = input.reason.trim(), classification = level, project_id = input.project || null;
    if (kind === 'contract') {
      const content = contractContent(input, evidence);
      const body: Schemas['ContractCreate'] | Schemas['ContractPatch'] = value ? { ...content, expected_version: value.version, base_revision_id: value.current_revision_id, reason } : { ...content, project_id, classification, parent_contract_id: input.parent || null, reason };
      void command.submit(businessRoutes[kind] + (value ? '/' + value.id : ''), body, value ? 'PATCH' : 'POST');
    } else {
      const content = spendContent(input, evidence);
      const body: Schemas['SpendCreate'] | Schemas['SpendPatch'] = value ? { ...content, expected_version: value.version, base_revision_id: value.current_revision_id, reason } : { ...content, project_id, classification, reason };
      void command.submit(businessRoutes[kind] + (value ? '/' + value.id : ''), body, value ? 'PATCH' : 'POST');
    }
  }
  const text = (name: keyof Omit<BusinessInput, 'kind' | 'lines' | 'plans'>, label: string, max = 200, multiline = false, hint?: string) => <TextField name={name} label={label} value={input[name]} onChange={value => change({ [name]: name === 'currency' ? value.toUpperCase() : value })} disabled={disabled} max={max} multiline={multiline} issues={validation ? issues : []} focus={focus} hint={hint} />;
  const select = (name: keyof Omit<BusinessInput, 'kind' | 'lines' | 'plans'>, label: string, options: readonly (readonly [string, string])[]) => <View id={'oa-' + name}><SelectField label={label} value={input[name]} options={options} onChange={value => change({ [name]: value })} disabled={disabled} />{issues.filter(issue => issue.path === name).map(issue => <Text key={issue.path} className='cp-field-error'>{issue.message}</Text>)}</View>;
  function files(files: Schemas['BusinessFileReference'][], remove: (id: string) => void) {
    return files.length ? files.map(file => <View className='cp-fixed-file' key={file.version_id}><Text className='cp-row-title'>{file.name} · 固定版本 {file.seq}</Text><State value={file.scan_state} /><Text className='cp-code'>{file.sha256}</Text><Action secondary disabled={disabled} onClick={() => remove(file.version_id)}>移除此固定依据</Action></View>) : <Empty>尚未选择固定版本。</Empty>;
  }
  function clearEvidence() { setEvidence(businessEvidence()); }
  if (picking) {
    const close = () => setPicking(undefined);
    if (picking === 'project') return <ChoiceList<Schemas['Project']> title='选择关联项目' path='/projects?limit=20' label={item => item.name} selected={input.project} eligible={item => !['closing', 'archived', 'cancelled'].includes(item.state)} onSelect={project => { scope({ project: project.id, parent: '' }); close(); }} onClose={close} />;
    if (picking === 'owner') return <ChoiceList<Schemas['User']> title='选择业务负责人' path='/users?limit=20' searchable={false} label={item => item.display_name} selected={input.owner} eligible={item => item.status === 'active'} onSelect={user => { change({ owner: user.id }); close(); }} onClose={close} />;
    if (picking === 'parent') return <ChoiceList<Schemas['Contract']> title='选择主合同' path={'/contracts?limit=20&lifecycle=effective' + (input.project ? '&project_id=' + input.project : '&company_only=true')} label={item => item.title} eligible={item => !item.deleted_at} onSelect={parent => { scope({ parent: parent.id, project: parent.project_id ?? '', classification: parent.classification }); close(); }} onClose={close} />;
    if (picking === 'body' || picking === 'quote' || typeof picking === 'object' && 'receipt' in picking) return <FileChoice project={input.project} classification={level} onClose={close} onSelect={file => {
      if (picking === 'body') setEvidence(old => ({ ...old, body: file }));
      else if (picking === 'quote') setEvidence(old => ({ ...old, quotes: [...old.quotes.filter(item => item.version_id !== file.version_id), file].slice(0, 20) }));
      else if (typeof picking === 'object' && 'receipt' in picking) setEvidence(old => ({ ...old, receipts: { ...old.receipts, [picking.receipt]: [...(old.receipts[picking.receipt] ?? []).filter(item => item.version_id !== file.version_id), file].slice(0, 20) } }));
      close();
    }} />;
    return <PartyChoice project={input.project} classification={level} onClose={close} onSelect={party => {
      const supplier = { party_id: party.id, revision_id: party.current_revision_id, name: party.name, kind: party.kind };
      if (picking === 'counterparty') setEvidence(old => ({ ...old, counterparty: { id: party.id, revision_id: party.current_revision_id, name: party.name, kind: party.kind } }));
      else if (picking === 'candidate') setEvidence(old => ({ ...old, candidates: [...old.candidates.filter(item => item.party_id !== party.id || item.revision_id !== party.current_revision_id), supplier].slice(0, 20) }));
      else if (typeof picking === 'object' && 'supplier' in picking) setEvidence(old => ({ ...old, lineSuppliers: { ...old.lineSuppliers, [picking.supplier]: supplier } }));
      close();
    }} />;
  }
  return <>
    <Text className='cp-muted cp-small cp-paragraph'>可先保存不完整的业务草稿；送审前由服务器核对完整资料和审批制度。尚未保存的输入仅在本次登录的有界内存中保留。</Text>
    <DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    {!draft.retained && <View className='cp-notice cp-danger'><Text>当前输入未进入会话草稿缓存，可能已超过共享的10万字符容量。当前页面仍保留输入，请先保存业务草稿；离开或刷新后不保证恢复。</Text></View>}
    {issues.length > 0 && <View className='cp-notice cp-danger' {...semantic('alert')}><Text className='cp-row-title'>有 {issues.length} 处需要补充或修正</Text>{issues.map(issue => <Action secondary key={issue.path} onClick={() => locate(issue)}>{issue.label}：{issue.message}</Action>)}</View>}
    {evidence.unresolved && <View className='cp-notice cp-danger'><Text>原修订包含当前无法完整读取的引用，保存已停止，避免静默移除依据。请刷新或明确清除全部引用后重新选择。</Text><Action secondary disabled={disabled} onClick={clearEvidence}>清除当前全部往来方与文件引用</Action></View>}
    <Section title='基本资料'><View className='cp-pad'>
      {text('title', businessNames[kind] + '标题')}{text('category', '业务类别', 50)}
      {!value ? <><View id='oa-project'><Fact label='关联项目'><Text>{input.project ? project.value?.name ?? '已选择项目，当前名称尚未取得' : '公司事项'}</Text><Text className='cp-muted cp-small'>变更项目或分类会清除已选往来方与文件依据。</Text></Fact></View><Notice error={project.error} />
        {me.actions.includes('project.read') && <Action secondary disabled={disabled || !!input.parent} onClick={() => setPicking('project')}>选择关联项目</Action>}
        {input.project && <Action secondary disabled={disabled || !!input.parent} onClick={() => scope({ project: '', parent: '', classification: level === 'project_restricted' ? 'internal' : level })}>改为公司事项</Action>}
        <SelectField label='资料分类' value={input.classification} options={input.project ? [['internal', '公司内部'], ['project_restricted', '项目受限'], ['confidential', '机密']] : [['internal', '公司内部'], ['confidential', '机密']]} disabled={disabled || !!input.parent} onChange={classification => scope({ classification })} />
        {kind === 'contract' && <><Fact label='补充协议归属'><Text>{input.parent ? (parent.value?.title ?? '已选主合同，当前名称尚未取得') + '；继承其项目与分类' : '独立合同'}</Text></Fact><Notice error={parent.error} /><Action secondary disabled={disabled} onClick={() => setPicking('parent')}>选择主合同，起草补充协议</Action>{input.parent && <Action secondary disabled={disabled} onClick={() => scope({ parent: '' })}>取消主合同关联</Action>}</>}
      </> : <Text className='cp-muted cp-small cp-paragraph'>此业务的项目归属与资料分类保持创建时的记录。</Text>}
      <View id='oa-owner'><Fact label='业务负责人'><Text>{input.owner === me.user.id ? me.user.display_name + '（本人）' : input.owner ? '已指定负责人' : '尚未指定'}</Text></Fact></View>
      <View className='cp-choice'><Action secondary disabled={disabled} onClick={() => change({ owner: me.user.id })}>指定本人负责</Action><Action secondary disabled={disabled} onClick={() => change({ owner: '' })}>暂不指定负责人</Action></View>
      {me.actions.includes('user.read') && <Action secondary disabled={disabled} onClick={() => setPicking('owner')}>选择其他负责人</Action>}
      {text('currency', kind === 'contract' ? '合同币种' : '币种', 3, false, '例如 HKD、CNY、USD、JPY；未知可留空。金额以原币字符串保存，不在本机四舍五入。')}
    </View></Section>
    {kind === 'contract' ? <>
      <Section title='合同内容'><View className='cp-pad'>
        {select('direction', '收付方向', [['', '稍后确认'], ['receivable', '收款合同'], ['payable', '付款合同'], ['none', '无收付义务']])}
        {text('contractType', '合同类型')}{text('amount', '合同总额', 30)}{text('tax', '其中税额', 30, false, '税额包含在合同总额中。币种精度与金额关系由服务器核对。')}
        {text('start', '起始日期', 10, false, 'YYYY-MM-DD，未知可留空。')}{text('end', '结束日期', 10, false, 'YYYY-MM-DD，未知可留空。')}
        <View id='oa-contract-evidence'><Fact label='固定往来方'><Text>{evidence.counterparty?.name ?? '尚未选择'}</Text></Fact></View>
        {me.actions.includes('party.read') && <Action secondary disabled={disabled} onClick={() => setPicking('counterparty')}>选择往来方修订</Action>}
        {evidence.counterparty && <Action secondary disabled={disabled} onClick={() => setEvidence(old => ({ ...old, counterparty: null }))}>移除往来方引用</Action>}
        {files(evidence.body ? [evidence.body] : [], () => setEvidence(old => ({ ...old, body: null })))}
        {me.actions.includes('file.read') && <Action secondary disabled={disabled} onClick={() => setPicking('body')}>选择合同正文版本</Action>}
      </View></Section>
      <View id='oa-schedule'><Section title='收付款计划'><View className='cp-pad'>
        <SelectField label='计划情况' value={input.scheduleMode} options={[[ '', '尚未整理' ], [ 'scheduled', '按节点收款或付款' ], [ 'no_payment', '明确无收付款义务' ]]} disabled={disabled} onChange={scheduleMode => {
          change({ scheduleMode, plans: scheduleMode === 'scheduled' ? input.plans.length ? input.plans : [newBusinessPlan(formID(client.operationKey()))] : [] });
        }} />
        {input.scheduleMode && text('scheduleReason', input.scheduleMode === 'no_payment' ? '无收付款义务依据' : '计划说明', 20000, true)}
        {input.scheduleMode === 'scheduled' && <>
          <Text className='cp-muted cp-small cp-paragraph'>所有节点均使用合同币种，送审时节点合计须与合同总额一致。</Text>
          {input.plans.map((plan, index) => <View key={plan.id} className='cp-editor-row'><Action secondary disabled={disabled} onClick={() => setPlanID(plan.id)}>节点 {index + 1} · {plan.title || '待填写'}</Action>
            {(planID === plan.id || !planID && index === 0) && <View>
              {(['title', 'amount', 'due', 'conditions'] as const).map((name, i) => <TextField key={name} name={'plan-' + plan.id + '-' + name} label={'节点 ' + (index + 1) + [' 名称', ' 金额', ' 到期日', ' 满足条件'][i]} value={plan[name]} onChange={value => changePlan(plan.id, { [name]: value })} disabled={disabled} max={name === 'conditions' ? 20000 : name === 'title' ? 200 : 30} multiline={name === 'conditions'} issues={issues} focus={focus} />)}
              <Action secondary disabled={disabled || input.plans.length === 1} onClick={() => { change({ plans: input.plans.filter(item => item.id !== plan.id) }); setPlanID(undefined); }}>移除此收付款节点</Action>
            </View>}
          </View>)}
          <Action secondary disabled={disabled || input.plans.length >= 100} onClick={() => { const plan = newBusinessPlan(formID(client.operationKey())); change({ plans: [...input.plans, plan] }); setPlanID(plan.id); }}>添加收付款节点</Action>
        </>}
      </View></Section></View>
      <Section title='条款与说明'><View className='cp-pad'>{text('renewal', '续约条款', 20000, true)}{text('termination', '终止条款', 20000, true)}</View></Section>
    </> : <>
      <Section title='用途与计价'><View className='cp-pad'>
        {text('purpose', '业务用途', 4000, true)}
        {select('pricing', '计价方式', kind === 'purchase' ? [['', '稍后确认'], ['net_plus_tax', '单价未含税，税额另加'], ['gross_includes_tax', '金额已含税，税额作说明']] : [['', '稍后确认'], ['gross_includes_tax', '金额已含税，税额作说明']])}
        {select('rounding', '舍入规则', [['', '稍后确认'], ['half_up', '四舍五入'], ['half_even', '五成双']])}
        {kind === 'purchase' ? <>{text('delivery', '预计交付日期', 10, false, 'YYYY-MM-DD')}{text('budget', '预算依据', 500)}
          {evidence.candidates.map(party => <View className='cp-fixed-file' key={party.party_id + party.revision_id}><Text>{party.name} · 已固定修订</Text><Action secondary disabled={disabled} onClick={() => setEvidence(old => ({ ...old, candidates: old.candidates.filter(item => item !== party) }))}>移除此候选供应商</Action></View>)}
          {me.actions.includes('party.read') && <Action secondary disabled={disabled || evidence.candidates.length >= 20} onClick={() => setPicking('candidate')}>添加候选供应商</Action>}
          {files(evidence.quotes, id => setEvidence(old => ({ ...old, quotes: old.quotes.filter(file => file.version_id !== id) })))}
          {me.actions.includes('file.read') && <Action secondary disabled={disabled || evidence.quotes.length >= 20} onClick={() => setPicking('quote')}>添加报价版本</Action>}
        </> : <>{text('periodStart', '费用期间开始', 10, false, 'YYYY-MM-DD')}{text('periodEnd', '费用期间结束', 10, false, 'YYYY-MM-DD')}</>}
      </View></Section>
      <View id='oa-lines'><Section title={businessNames[kind] + '明细 · ' + input.lines.length + '/100'}><View className='cp-pad'>
        {!input.lines.length && <Empty>可先保存草稿，或添加明细继续整理。</Empty>}
        {input.lines.map((line, index) => <View className='cp-editor-row' key={line.id}><Action secondary disabled={disabled} onClick={() => setLineID(line.id)}>明细 {index + 1} · {line.description || '待填写'}</Action>
          {lineID === line.id && <View>
            {(['description', 'quantity', 'unit', 'price', ...(kind === 'claim' ? ['amount'] : []), 'tax', ...(kind === 'purchase' ? ['discount'] : [])] as (keyof BusinessLine)[]).map(name => <TextField key={name} name={'line-' + line.id + '-' + name} label={'明细 ' + (index + 1) + ' ' + ({ description: '描述', quantity: '数量', unit: '单位', price: '单价', amount: '实际含税费用', tax: input.pricing === 'net_plus_tax' ? '另加税额' : '其中税额', discount: '折扣金额' } as Record<string, string>)[name]} value={line[name]} disabled={disabled} max={name === 'description' ? 1000 : name === 'unit' ? 50 : 30} onChange={value => changeLine(line.id, { [name]: value })} issues={issues} focus={focus} />)}
            {kind === 'claim' && <>
              <TextField name={'line-' + line.id + '-date'} label={'明细 ' + (index + 1) + ' 费用日期'} value={line.date} onChange={date => changeLine(line.id, { date })} max={10} disabled={disabled} hint='YYYY-MM-DD' issues={issues} focus={focus} />
              <TextField name={'line-' + line.id + '-category'} label={'明细 ' + (index + 1) + ' 费用类别'} value={line.category} onChange={category => changeLine(line.id, { category })} max={50} disabled={disabled} />
              <SelectField label={'明细 ' + (index + 1) + ' 资金来源'} value={line.funding} options={[[ '', '稍后确认' ], [ 'employee_advance', '员工垫付' ], [ 'company_paid', '公司已付' ]]} onChange={funding => changeLine(line.id, { funding })} disabled={disabled} />
              <Fact label='本行供应商'><Text>{evidence.lineSuppliers[line.id]?.name ?? '尚未选择'}</Text></Fact>
              {me.actions.includes('party.read') && <Action secondary disabled={disabled} onClick={() => setPicking({ supplier: line.id })}>选择本行供应商</Action>}
              {evidence.lineSuppliers[line.id] && <Action secondary disabled={disabled} onClick={() => setEvidence(old => ({ ...old, lineSuppliers: { ...old.lineSuppliers, [line.id]: undefined } }))}>移除本行供应商</Action>}
              {files(evidence.receipts[line.id] ?? [], id => setEvidence(old => ({ ...old, receipts: { ...old.receipts, [line.id]: (old.receipts[line.id] ?? []).filter(file => file.version_id !== id) } })))}
              {me.actions.includes('file.read') && <Action secondary disabled={disabled || (evidence.receipts[line.id]?.length ?? 0) >= 20} onClick={() => setPicking({ receipt: line.id })}>添加本行费用凭证</Action>}
              {(['missing', 'duplicate'] as const).map((name, i) => <TextField key={name} name={'line-' + line.id + '-' + name} label={'明细 ' + (index + 1) + [' 无凭证说明', ' 重复凭证说明'][i]} value={line[name]} onChange={value => changeLine(line.id, { [name]: value })} max={2000} multiline disabled={disabled} />)}
            </>}
            <Action secondary disabled={disabled} onClick={() => { change({ lines: input.lines.filter(item => item.id !== line.id) }); setEvidence(old => { const { [line.id]: supplier, ...lineSuppliers } = old.lineSuppliers, { [line.id]: receipt, ...receipts } = old.receipts; return { ...old, lineSuppliers, receipts }; }); setLineID(undefined); }}>移除此明细</Action>
          </View>}
        </View>)}
        <Action secondary disabled={disabled || input.lines.length >= 100} onClick={() => { const line = newBusinessLine(formID(client.operationKey())); change({ lines: [...input.lines, line] }); setLineID(line.id); }}>添加明细</Action>
      </View></Section></View>
    </>}
    <View id='oa-save'><Section title='保存业务草稿'><View className='cp-pad'>{text('notes', kind === 'contract' ? '补充说明' : '备注', 20000, true)}{text('reason', value ? '本次修改依据' : '创建依据', 200, true)}
      <Text className='cp-muted cp-small cp-paragraph'>保存仅生成业务修订。之后还需明确建立申请、核对预览并送审。</Text>
      <Action disabled={disabled || evidence.unresolved} loading={command.busy} onClick={save}>{value ? '保存为新的业务修订' : '保存' + businessNames[kind] + '草稿'}</Action>
      <Action secondary disabled={command.busy} onClick={onCancel}>返回，保留未提交输入</Action>
    </View></Section></View>
  </>;
}
