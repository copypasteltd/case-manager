import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fixedDeliveryNavigation, fixedDeliveryStateNames, matchingNotificationDelivery, organizationOutcomeTarget, organizationPlanSourceMatches, type NotificationDeliveryNavigation, type OrdinaryFileTarget } from '@copypaste/contracts/fixed-delivery-navigation';
import { api, type Me, type Schemas } from './api';
import { Busy, date, ErrorNotice } from './components';

const filePath = (target: OrdinaryFileTarget) => target.kind === 'file' ? '/files/' + target.id : '/files/folders/' + target.id;

export function FixedDeliveryNavigation({ run }: { run: Schemas['Run'] }) {
  let receipt;
  try { receipt = fixedDeliveryNavigation(run); } catch (error) { return <ErrorNotice error={error} />; }
  if (!receipt) return null;
  return <section className="conversation-source-preview"><h3>{receipt.kind === 'scope_transfer' ? '准确范围计划回执' : '准确通知片段回执'}</h3>
    <p>固定授权哈希</p><code className="full-digest">{receipt.intentSHA256}</code>
    {receipt.operationID && <p>普通业务操作编号 <code>{receipt.operationID}</code></p>}
    {receipt.kind === 'scope_transfer' ? <><p>计划编号 <code>{receipt.planID}</code> · 授权修订 {receipt.authorizedVersion}</p><code className="full-digest">{receipt.planHash}</code>
      <p>{receipt.executed ? '普通范围变更已执行，旧秘书授权已失效。此回执仅保留准确执行身份；当前资料按现有权限重新查阅。' : '本次已固定准确范围计划，尚无普通执行回执。'}</p>
      {receipt.source && <Link className="button secondary" to={filePath(receipt.source)}>查看当前有权源资料</Link>}{receipt.target && <Link className="button secondary" to={filePath(receipt.target)}>查看当前有权目标目录</Link>}
      <Link className="button secondary" to="/files">进入当前文件工作区</Link>
    </> : <NotificationReceipt receipt={receipt} />}
  </section>;
}

function NotificationReceipt({ receipt }: { receipt: NotificationDeliveryNavigation }) {
  const [open, setOpen] = useState(false);
  const query = useQuery({ queryKey: ['fixed-notification-delivery', receipt.runID, receipt.intentSHA256, receipt.requestID, receipt.deliveryID], enabled: open && !!receipt.deliveryID, gcTime: 0, retry: false,
    queryFn: async ({ signal }) => matchingNotificationDelivery(await api<Schemas['TelegramDeliveryDetails']>('/me/telegram/requests/' + receipt.requestID + '/deliveries', { signal }), receipt) });
  const current = query.isSuccess && !query.isFetching ? query.data : undefined;
  return <><p>原请求 <code>{receipt.requestID}</code> · 原结果任务 <code>{receipt.sourceRunID}</code></p><p>已授权原结果修订 {receipt.authorizedVersion} · 字符起点 {receipt.startChar} · 长度 {receipt.maxChars}</p><code className="full-digest">{receipt.bodySHA256}</code>
    {receipt.deliveryID ? <><p>人工披露回执 <code>{receipt.reviewID}</code> · 投递编号 <code>{receipt.deliveryID}</code></p><p>通知已进入原群投递队列；实际发送和不确定结果以当前投递账本为准。</p>
      <button type="button" className="button secondary" onClick={() => setOpen(value => !value)}>{open ? '收起本条投递状态' : '核对本条实际投递状态'}</button>
      {open && <><ErrorNotice error={query.error} />{query.isPending || query.isFetching ? <Busy label="正在核对准确投递记录" /> : current && <><p>当前状态：<strong>{fixedDeliveryStateNames[current.delivery.state]}</strong> · 尝试 {current.delivery.attempts} 次 · {date(current.delivery.updated_at)}</p>{current.delivery.message_id !== null && <p>平台消息编号 {current.delivery.message_id}</p>}<ol>{current.events.map(event => <li key={event.id}>修订 {event.version} · {fixedDeliveryStateNames[event.state]} · {date(event.created_at)}</li>)}</ol></>}
        <button type="button" className="button secondary" onClick={() => void query.refetch()}>重新核对本条投递</button></>}
    </> : <p>本次通知片段已固定，尚无人工披露与投递回执。</p>}
    <Link className="button secondary" to={'/telegram?request=' + receipt.requestID}>查看原请求与投递账本</Link>
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

export function OrganizationPlanSourceNavigation({ reference, conversation, me, onClose }: { reference: Schemas['AgentSourceSnapshot']; conversation: Schemas['Conversation']; me: Me; onClose(): void }) {
  const query = useQuery({ queryKey: ['organization-source-navigation', conversation.id, conversation.context_security_version, me.user.id, me.authorization_version, reference.id, reference.sha256], gcTime: 0, retry: false,
    queryFn: async ({ signal }) => {
      const plan = await api<Schemas['AgentOrganizationPlan']>('/organization-plans/' + reference.id, { signal });
      if (!organizationPlanSourceMatches(plan, reference, conversation.id, me.user.id)) throw new Error('引用与当前准确整理计划不一致，请重新核对。');
      return plan;
    } });
  const plan = query.isSuccess && !query.isFetching ? query.data : undefined;
  return <section className="panel conversation-summary"><h2>引用的准确资料整理计划</h2><ErrorNotice error={query.error} />{query.isPending || query.isFetching ? <Busy /> : plan && <>
    <p><strong>{planStates[plan.state]}</strong> · 修订 {plan.version} · <code>{plan.id}</code></p><code className="full-digest">{plan.plan_hash}</code><p>{plan.reason}</p><p>到期 {date(plan.expires_at)}；此处只查阅当前有权计划，执行仍需独立批准。</p>{plan.blocked_reason && <p>{plan.blocked_reason}</p>}
    <ol>{plan.preview.map((item, index) => { const outcome = plan.results[index], result = organizationOutcomeTarget(plan, index); return <li key={index}><p>{actionNames[item.action.kind]} · {item.source.name}</p><p>原位置：{location(item.path, item.source)}<br />变更后：{destination(item)}<br />资料分类：{classificationNames[item.source.classification]}</p><p>源 <code>{item.source.id}</code> · 修订 {item.source.version}</p>{item.target && <p>目标 <code>{item.target.id}</code> · 修订 {item.target.version}</p>}
      {outcome && <p>普通操作 <code>{outcome.operation_id}</code> · {outcome.data.name}</p>}{result && <Link className="button secondary" to={filePath(result)}>查看本项当前有权结果</Link>}
    </li>; })}</ol>{plan.operation_id && <p>整份计划操作编号 <code>{plan.operation_id}</code></p>}
  </>}<button type="button" className="button secondary" onClick={() => void query.refetch()}>重新读取准确计划</button><button type="button" className="button secondary" onClick={onClose}>关闭引用计划</button></section>;
}
