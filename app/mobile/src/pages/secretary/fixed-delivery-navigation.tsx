import { useState } from 'react';
import { Text, View } from '@tarojs/components';
import Taro, { useDidHide } from '@tarojs/taro';
import { fixedDeliveryNavigation, fixedDeliveryStateNames, matchingNotificationDelivery, organizationOutcomeTarget, organizationPlanSourceMatches, type NotificationDeliveryNavigation } from '@copypaste/contracts/fixed-delivery-navigation';
import type { Schemas } from '../../platform/session';
import { useRecord } from '../../platform/collection';
import { go } from '../../platform/page';
import { Action, Busy, Fact, Notice, Section, date } from '../../ui';

export function FixedDeliveryNavigation({ run }: { run: Schemas['Run'] }) {
  let receipt;
  try { receipt = fixedDeliveryNavigation(run); } catch (error) { return <Notice error={error} />; }
  if (!receipt) return null;
  return <View className='cp-editor-row'><Text className='cp-row-title'>{receipt.kind === 'scope_transfer' ? '准确范围计划回执' : '准确通知片段回执'}</Text>
    <Fact label='固定授权哈希'><Text className='cp-code' selectable>{receipt.intentSHA256}</Text></Fact>{receipt.operationID && <Fact label='普通业务操作编号'><Text selectable>{receipt.operationID}</Text></Fact>}
    {receipt.kind === 'scope_transfer' ? <><Fact label={'计划编号 · 授权修订 ' + receipt.authorizedVersion}><Text selectable>{receipt.planID}</Text></Fact><Text className='cp-code' selectable>{receipt.planHash}</Text>
      <Text className='cp-paragraph'>{receipt.executed ? '普通范围变更已执行，旧秘书授权已失效。当前资料按现有权限重新查阅。' : '本次已固定准确范围计划，尚无普通执行回执。'}</Text>
      {receipt.source && <Action secondary onClick={() => go(receipt.source!.kind, receipt.source!.id)}>查看当前有权源资料</Action>}{receipt.target && <Action secondary onClick={() => go(receipt.target!.kind, receipt.target!.id)}>查看当前有权目标目录</Action>}
      <Action secondary onClick={() => go('files')}>进入当前文件工作区</Action>
    </> : <NotificationReceipt key={receipt.runID + ':' + receipt.deliveryID} receipt={receipt} />}
  </View>;
}

function NotificationReceipt({ receipt }: { receipt: NotificationDeliveryNavigation }) {
  const [open, setOpen] = useState(false);
  useDidHide(() => setOpen(false));
  const query = useRecord<Schemas['TelegramDeliveryDetails']>(open && receipt.deliveryID ? '/me/telegram/requests/' + receipt.requestID + '/deliveries' : null);
  let current, error = query.error;
  if (open && !query.loading && !error && query.value) {
    try { current = matchingNotificationDelivery(query.value, receipt); } catch (cause) { error = cause; }
  }
  return <><Fact label='原请求'><Text selectable>{receipt.requestID}</Text></Fact><Fact label='原结果任务'><Text selectable>{receipt.sourceRunID}</Text></Fact><Text className='cp-paragraph'>授权原结果修订 {receipt.authorizedVersion} · 字符起点 {receipt.startChar} · 长度 {receipt.maxChars}</Text><Text className='cp-code' selectable>{receipt.bodySHA256}</Text>
    {receipt.deliveryID ? <><Fact label='人工披露回执'><Text selectable>{receipt.reviewID}</Text></Fact><Fact label='准确投递编号'><Text selectable>{receipt.deliveryID}</Text></Fact><Text className='cp-paragraph'>通知已进入原群投递队列；实际发送和不确定结果以当前投递账本为准。</Text>
      <Action secondary onClick={() => setOpen(value => !value)}>{open ? '收起本条投递状态' : '核对本条实际投递状态'}</Action>{open && <><Notice error={error} />{query.loading ? <Busy /> : current && <><Fact label='当前实际状态'><Text>{fixedDeliveryStateNames[current.delivery.state]} · 尝试 {current.delivery.attempts} 次 · {date(current.delivery.updated_at)}</Text></Fact>{current.delivery.message_id !== null && <Fact label='平台消息编号'><Text>{current.delivery.message_id}</Text></Fact>}{current.events.map(event => <Fact key={event.id} label={'修订 ' + event.version}><Text>{fixedDeliveryStateNames[event.state]} · {date(event.created_at)}</Text></Fact>)}</>}<Action secondary onClick={query.reload}>重新核对本条投递</Action></>}
    </> : <Text className='cp-paragraph'>本次通知片段已固定，尚无人工披露与投递回执。</Text>}
    <Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/telegram/index?request=' + receipt.requestID })}>查看原请求与投递账本</Action>
  </>;
}

const planStates: Record<Schemas['AgentOrganizationPlan']['state'], string> = { proposed: '待批准', approved: '已批准', revoked: '已撤销', committed: '已执行' };
const actionNames: Record<Schemas['AgentOrganizationAction']['kind'], string> = { 'folder.create': '新建目录', 'folder.move': '移动目录', 'file.move': '移动文件' };
const classificationNames = { internal: '公司内部', project_restricted: '项目受限', confidential: '严格受限' };
const location = (path: Schemas['AgentOrganizationEntry'][], entry: Schemas['AgentOrganizationEntry']) => [...path].reverse().map(value => value.name).concat(entry.name).join(' / ');
function destination(item: Schemas['AgentOrganizationPreview']) {
  if (item.action.kind === 'folder.create') return location(item.path, item.source) + ' / ' + item.action.name;
  return item.target ? location(item.target_path, item.target) + ' / ' + (item.action.name ?? item.source.name) : [...item.path].reverse().map(value => value.name).concat(item.action.name ?? item.source.name).join(' / ');
}

export function OrganizationPlanSourceNavigation({ reference, conversation, me, onClose }: { reference: Schemas['AgentSourceSnapshot']; conversation: Schemas['Conversation']; me: Schemas['Me']; onClose(): void }) {
  const query = useRecord<Schemas['AgentOrganizationPlan']>('/organization-plans/' + reference.id);
  let plan = query.loading || query.error ? undefined : query.value;
  const mismatch = plan && !organizationPlanSourceMatches(plan, reference, conversation.id, me.user.id);
  if (mismatch) plan = undefined;
  return <Section title='引用的准确资料整理计划'><View className='cp-pad'><Notice error={query.error ?? (mismatch ? new Error('引用与当前准确整理计划不一致，请重新核对。') : undefined)} />{query.loading ? <Busy /> : plan && <>
    <Fact label={planStates[plan.state] + ' · 修订 ' + plan.version}><Text selectable>{plan.id}</Text></Fact><Text className='cp-code' selectable>{plan.plan_hash}</Text><Text className='cp-paragraph'>{plan.reason}</Text><Text className='cp-paragraph'>到期 {date(plan.expires_at)}；此处只查阅当前有权计划，执行仍需独立批准。</Text>{plan.blocked_reason && <Text>{plan.blocked_reason}</Text>}
    {plan.preview.map((item, index) => { const outcome = plan!.results[index], result = organizationOutcomeTarget(plan!, index); return <View className='cp-editor-row' key={index}><Text className='cp-row-title'>{index + 1}. {actionNames[item.action.kind]} · {item.source.name}</Text><Fact label='原位置'><Text>{location(item.path, item.source)}</Text></Fact><Fact label='变更后'><Text>{destination(item)}</Text></Fact><Fact label='资料分类'><Text>{classificationNames[item.source.classification]}</Text></Fact><Fact label={'源 · 修订 ' + item.source.version}><Text selectable>{item.source.id}</Text></Fact>{item.target && <Fact label={'目标 · 修订 ' + item.target.version}><Text selectable>{item.target.id}</Text></Fact>}{outcome && <Fact label={'普通操作 · ' + outcome.data.name}><Text selectable>{outcome.operation_id}</Text></Fact>}{result && <Action secondary onClick={() => go(result.kind, result.id)}>查看本项当前有权结果</Action>}</View>; })}
    {plan.operation_id && <Fact label='整份计划操作编号'><Text selectable>{plan.operation_id}</Text></Fact>}
  </>}<Action secondary onClick={query.reload}>重新读取准确计划</Action><Action secondary onClick={onClose}>关闭引用计划</Action></View></Section>;
}
