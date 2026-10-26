import { Text, View } from '@tarojs/components';
import { Button } from '../../platform/components';
import { go, goVersion } from '../../platform/page';
import type { Schemas } from '../../platform/session';
import { Empty, Fact, Section, State, bytes } from '../../ui';

const directions = { receivable: '收款合同', payable: '付款合同', none: '无收付义务' };
const classifications = { internal: '公司内部', project_restricted: '项目受限', confidential: '机密' };

function SpendSnapshot({ snapshot, digest, heading = '本次申请的固定内容' }: { snapshot: Schemas['SpendRequestPayload']; digest?: string; heading?: string }) {
  const c = snapshot.content, purchase = snapshot.kind === 'purchase';
  const amount = (v: string | null | undefined) => v == null ? '未填写' : (c.currency ?? '币种未设置') + ' ' + v;
  const supplier = (ref: Schemas['SpendSupplier']) => snapshot.suppliers.find(item => item.party_id === ref.party_id && item.revision_id === ref.revision_id)?.name ?? '已固定资料，当前不可查阅';
  return <><Section title={heading}><Fact label={purchase ? '采购编号' : '报销编号'}><Text>{snapshot.document_no}</Text></Fact><Fact label='业务名称'><Text>{c.title}</Text></Fact><Fact label='业务类别'><Text>{c.category || '未填写'}</Text></Fact><Fact label='业务用途'><Text className='cp-paragraph'>{c.purpose || '未填写'}</Text></Fact><Fact label='资料分类'><Text>{classifications[snapshot.classification]}</Text></Fact><Fact label='关联项目'>{snapshot.project_id ? <Button className='cp-link' onClick={() => go('project', snapshot.project_id!)}>查阅所属项目 ↗</Button> : <Text>公司事项</Text>}</Fact><Fact label='负责人'><Text>{c.owner_id ? '已指定负责人' : '未填写'}</Text></Fact><Fact label={purchase ? '预计采购总额' : '费用合计'}><Text>{amount(snapshot.amount)}</Text></Fact>{!purchase && <Fact label='其中员工垫付'><Text>{amount(snapshot.employee_reimbursement_amount)}</Text></Fact>}<Fact label='计价方式'><Text>{c.pricing_basis === 'net_plus_tax' ? '单价未含税，税额另加' : c.pricing_basis === 'gross_includes_tax' ? '金额已含税，税额作说明' : '未填写'}</Text></Fact><Fact label='舍入规则'><Text>{c.rounding_mode === 'half_up' ? '四舍五入' : c.rounding_mode === 'half_even' ? '五成双' : '未填写'}</Text></Fact>{purchase ? <><Fact label='预计交付日期'><Text>{c.expected_delivery_date ?? '未填写'}</Text></Fact><Fact label='预算依据'><Text>{c.budget_reference || '未填写'}</Text></Fact><Fact label='候选供应商'><Text>{c.supplier_candidates.length ? c.supplier_candidates.map(supplier).join('、') : '未填写'}</Text></Fact></> : <Fact label='费用期间'><Text>{c.period_start ?? '未填写'} — {c.period_end ?? '未填写'}</Text></Fact>}<Fact label='备注'><Text className='cp-paragraph'>{c.notes || '未填写'}</Text></Fact></Section>
    <Section title={purchase ? '采购明细' : '费用明细'}>{c.lines.length ? c.lines.map((line, index) => <View key={line.id} className='cp-pad'><Text className='cp-row-title'>{index + 1} · {line.description || '明细待补充'}</Text><Fact label='明细金额'><Text>{amount(snapshot.line_values.find(item => item.line_id === line.id)?.amount)}</Text></Fact>{line.quantity != null && <Fact label='数量'><Text>{line.quantity} {line.unit}</Text></Fact>}{line.unit_price != null && <Fact label='单价'><Text>{amount(line.unit_price)}</Text></Fact>}<Fact label={c.pricing_basis === 'net_plus_tax' ? '另加税额' : '其中税额'}><Text>{amount(line.tax_amount)}</Text></Fact>{purchase ? <Fact label='折扣金额'><Text>{amount(line.discount_amount)}</Text></Fact> : <><Fact label='费用日期'><Text>{line.expense_date ?? '未填写'}</Text></Fact><Fact label='费用类别'><Text>{line.category || '未填写'}</Text></Fact><Fact label='供应商'><Text>{line.supplier ? supplier(line.supplier) : '未填写'}</Text></Fact><Fact label='资金来源'><Text>{line.funding === 'employee_advance' ? '员工垫付' : line.funding === 'company_paid' ? '公司已付' : '未填写'}</Text></Fact><Fact label='凭证'><Text>{line.receipt_version_ids.length ? '已固定 ' + line.receipt_version_ids.length + ' 个版本' : '未选择'}</Text></Fact>{line.missing_receipt_reason && <Fact label='无凭证说明'><Text className='cp-paragraph'>{line.missing_receipt_reason}</Text></Fact>}{line.duplicate_receipt_reason && <Fact label='重复凭证说明'><Text className='cp-paragraph'>{line.duplicate_receipt_reason}</Text></Fact>}</>}</View>) : <Empty>尚未填写明细。</Empty>}</Section>
    <Section title='固定附件依据'>{snapshot.attachments.length ? snapshot.attachments.map(file => <PinnedFile key={file.version_id} file={file} />) : <Empty>当前没有可查阅的固定附件。</Empty>}</Section>{digest && <Section title='申请内容摘要'><Text className='cp-code' selectable>{digest}</Text></Section>}<Text className='cp-muted cp-small cp-paragraph'>审批确认本次业务内容，后续执行与实际付款分别登记。</Text></>;
}

function PinnedFile({ file, body = false }: { file: Schemas['BusinessFileReference']; body?: boolean }) {
  return <View className='cp-pad'><Text className='cp-row-title'>{file.name}</Text>
    <Text className='cp-muted cp-small'>固定版本 {file.seq} · {bytes(file.size_bytes)}</Text>
    <State value={file.scan_state} />
    {file.scan_state === 'waived' && <Text className='cp-notice cp-paragraph'>此版本按扫描例外放行。请按公司约定核对原件风险。</Text>}
    <Text className='cp-muted cp-small'>SHA-256</Text><Text className='cp-code' selectable>{file.sha256}</Text>
    <Button className='cp-link' onClick={() => goVersion(file.file_id, file.version_id)}>{body ? '查看固定正文资料 ↗' : '查看此固定附件资料 ↗'}</Button>
  </View>;
}

function ChangeBudgetLines({ lines, currency }: { lines: Schemas['ProjectBudgetLine'][]; currency: string }) {
 return lines.length ? <>{lines.map((line,index)=><View className='cp-pad' key={index}><Text className='cp-row-title'>{line.category}</Text><Fact label='期间'><Text>{line.period_start} — {line.period_end}</Text></Fact><Fact label='原币预算'><Text selectable>{currency} {line.amount}</Text></Fact></View>)}</> : <Empty>此固定预算没有明细。</Empty>;
}
function ChangeSnapshot({ snapshot, digest }: { snapshot: Schemas['ChangeRequestPayload']; digest: string }) {
 const c=snapshot.content,b=snapshot.project_change_basis;
 return <><Section title='本次项目变更的固定基线'><Fact label='变更名称'><Text className='cp-paragraph'>{c.title}</Text></Fact><Fact label='业务类别'><Text>{c.category}</Text></Fact><Fact label='项目'><Text className='cp-paragraph'>{b.name} · 固定版本 {b.project_version}</Text></Fact><Fact label='原范围'><Text className='cp-paragraph'>{b.scope||'未填写'}</Text></Fact><Fact label='原起止日期'><Text>{b.start_date??'未设置'} — {b.due_date??'未设置'}</Text></Fact><Fact label='原预算总额'><Text selectable>{b.currency} {b.budget_total}</Text></Fact><Fact label='原预算标识'><Text className='cp-code' selectable>{b.budget_id??'尚无该币种的已实施预算'}</Text></Fact></Section>
 <Section title='完整拟实施内容'><Fact label='拟实施范围'><Text className='cp-paragraph'>{c.scope}</Text></Fact><Fact label='拟定起止日期'><Text>{c.start_date??'未设置'} — {c.due_date??'未设置'}</Text></Fact><Fact label='范围影响说明'><Text className='cp-paragraph'>{c.scope_delta}</Text></Fact><Fact label='时间影响说明'><Text className='cp-paragraph'>{c.schedule_delta}</Text></Fact><Fact label='预算净变化'><Text selectable>{c.currency} {snapshot.project_change_budget_delta}</Text></Fact><Fact label='审批规则使用的增加合计'><Text selectable>{c.currency} {snapshot.amount}</Text></Fact><Text className='cp-muted cp-small cp-paragraph'>明细减少不会抵消其他明细的增加；不同币种分别核对。</Text></Section>
 <Section title='原预算完整明细'><ChangeBudgetLines lines={b.budget_lines} currency={b.currency}/></Section><Section title='建议预算完整明细'>{c.budget_lines===null?<Text className='cp-paragraph'>保持该币种当前预算。</Text>:!c.budget_lines.length?<Text className='cp-paragraph'>本次将该币种预算明细清空。</Text>:<ChangeBudgetLines lines={c.budget_lines} currency={c.currency}/>}</Section>
 <Section title='固定依据文件'>{snapshot.attachments.length?snapshot.attachments.map(file=><PinnedFile key={file.version_id} file={file}/>):<Empty>尚无可查阅的固定附件。</Empty>}</Section><Section title='准确申请摘要'><Text className='cp-code' selectable>{digest}</Text></Section><Text className='cp-muted cp-small cp-paragraph'>独立批准后仍须由获授权人员明确实施。审批本身不会修改当前项目、预算、实际费用或付款。</Text></>;
}
export function RequestSnapshot({ request, me }: { request: Schemas['Request']; me: Schemas['Me'] }) {
  return <PayloadSnapshot snapshot={request.payload} digest={request.content_hash} me={me} steps={request.steps} />;
}
export function PayloadSnapshot({ snapshot, digest, me, steps = [], heading = '本次申请的固定内容' }: { snapshot: Schemas['RequestPayload']; digest: string; me: Schemas['Me']; steps?: Schemas['ApprovalTask'][]; heading?: string }) {
  if (snapshot.kind === 'change') return <ChangeSnapshot snapshot={snapshot} digest={digest} />;
  if (snapshot.kind === 'purchase_variance') {
    const basis = snapshot.purchase_variance;
    return <><Section title='采购差异重审'><Text className='cp-notice cp-paragraph'>此差异获批后，仍须由获授权人员明确启用。现有执行依据继续约束订单。</Text><Fact label='固定采购版本'><Text>{basis.purchase_version}</Text></Fact><Fact label='送审时执行依据'><Text>{basis.execution_plan_revision_id ? '此前已启用的差异基线' : '原采购商业批准'}</Text></Fact><Fact label='原采购批准摘要'><Text className='cp-code' selectable>{basis.original_content_hash}</Text></Fact></Section><Section title='送审时履行集合'>{basis.commitments.map(line => <View className='cp-pad' key={line.line_id}><Text className='cp-row-title'>{basis.content.lines.find(item => item.id === line.line_id)?.description ?? '固定采购行'}</Text><Fact label='当时执行数量'><Text>{line.approved_quantity}</Text></Fact><Fact label='已占用数量'><Text>{line.reserved_quantity}</Text></Fact><Fact label='净验收／待收数量'><Text>{line.net_accepted_quantity} ／ {line.outstanding_quantity}</Text></Fact><Fact label='已占用金额'><Text>{basis.content.currency} {line.reserved_amount}</Text></Fact><Fact label='已占用税额／折扣'><Text>{line.reserved_tax} ／ {line.reserved_discount}</Text></Fact></View>)}</Section><SpendSnapshot snapshot={{ ...snapshot, kind: 'purchase', content: basis.content, amount: basis.amount, line_values: basis.line_values }} heading='送审时完整执行依据' /><SpendSnapshot snapshot={{ ...snapshot, kind: 'purchase' }} heading='完整拟执行内容' digest={digest} /></>;
  }
  if (snapshot.kind !== 'contract') return <SpendSnapshot snapshot={snapshot} digest={digest} heading={heading} />;
  const content = snapshot.content;
  const body = snapshot.attachments.find(file => file.version_id === content.draft_file_version_id);
  const attachments = snapshot.attachments.filter(file => file.version_id !== content.draft_file_version_id);
  const owner = content.owner_id === me.user.id ? me.user.display_name : steps.find(step => step.assigned_user_id === content.owner_id)?.assigned_user_name;
  return <>
    <Section title={heading}>
      <Fact label='合同编号'><Text>{snapshot.contract_no}</Text></Fact>
      <Fact label='合同名称'><Text>{content.title}</Text></Fact>
      <Fact label='收付方向'><Text>{content.direction ? directions[content.direction] : '未填写'}</Text></Fact>
      <Fact label='合同类型'><Text>{content.type || '未填写'}</Text></Fact>
      <Fact label='业务类别'><Text>{content.category || '未填写'}</Text></Fact>
      <Fact label='资料分类'><Text>{classifications[snapshot.classification]}</Text></Fact>
      <Fact label='关联项目'>{snapshot.project_id ? <Button className='cp-link' onClick={() => go('project', snapshot.project_id!)}>查阅所属项目 ↗</Button> : <Text>公司事项，未关联项目。</Text>}</Fact>
      <Fact label='负责人'><Text>{owner ?? (content.owner_id ? '已指定负责人' : '未填写')}</Text>{owner && <Text className='cp-muted cp-small'>姓名按当前可见人员资料显示；责任人身份固定于本次修订。</Text>}</Fact>
      <Fact label='合同金额'><Text>{content.currency ?? '币种未设置'} {content.amount ?? '金额未设置'}</Text></Fact>
      <Fact label='其中税额'><Text>{content.tax_amount == null ? '未设置' : (content.currency ?? '币种未设置') + ' ' + content.tax_amount}</Text></Fact>
      <Fact label='往来方依据'><Text>{snapshot.counterparty?.name ?? (content.counterparty_id ? '已引用，当前不可查阅' : '未设置')}</Text>{snapshot.counterparty && <Text className='cp-muted cp-small'>{snapshot.counterparty.kind === 'organization' ? '组织' : '个人'} · 已固定资料修订</Text>}</Fact>
      <Fact label='起止日期'><Text>{content.start_date ?? '未设置'} — {content.end_date ?? '未设置'}</Text></Fact>
      <Fact label='续约约定'><Text className='cp-paragraph'>{content.renewal_terms || '未填写'}</Text></Fact>
      <Fact label='终止约定'><Text className='cp-paragraph'>{content.termination_terms || '未填写'}</Text></Fact>
      <Fact label='备注'><Text className='cp-paragraph'>{content.notes || '未填写'}</Text></Fact>
      <Fact label='申请内容摘要'><Text className='cp-code' selectable>{digest}</Text></Fact>
    </Section>
    <Section title='合同正文依据'>{body ? <PinnedFile file={body} body /> : <Empty>{content.draft_file_version_id ? '已引用正文版本，当前不可用或无权查阅。' : '此修订尚未选择正文。'}</Empty>}</Section>
    <Section title='收付计划'>{!content.payment_schedule ? <Empty>此修订尚未填写收付计划。</Empty> : <>
      <Fact label='计划类型'><Text>{content.payment_schedule.mode === 'no_payment' ? '无需收付' : '分期收付'}</Text><Text className='cp-paragraph'>{content.payment_schedule.reason || '未填写说明'}</Text></Fact>
      {content.payment_schedule.items.map(plan => <Fact key={plan.id} label={plan.title}><Text>{content.currency ?? '币种未设置'} {plan.amount ?? '金额未填写'} · {plan.due_date ?? '日期未填写'}</Text><Text className='cp-paragraph'>{plan.conditions || '未填写满足条件'}</Text></Fact>)}
    </>}</Section>
    <Section title='其他固定附件依据'>{attachments.length ? attachments.map(file => <PinnedFile key={file.version_id} file={file} />) : <Empty>此修订没有其他附件。</Empty>}</Section>
    <Text className='cp-muted cp-small cp-paragraph'>审批确认本次申请内容。签署、生效和实际收付款分别登记。</Text>
  </>;
}
