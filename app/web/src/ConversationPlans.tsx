import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type Schemas } from './api';
import { Busy, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

import { emptyProjectSelection, projectPlanProject, projectPlanMatches } from '@copypaste/contracts/project-catalog';
import { ProjectCatalogSelection } from './ConversationProjectCatalog';

const states = { proposed: '待本人批准', approved: '已批准，等待创建', committed: '项目已创建', revoked: '已撤销' };
const reasons: Record<string,string> = { PLAN_EXPIRED: '计划已过期，请重新拟定。', PLAN_CONTEXT_CHANGED: '会话已变化，请重新拟定。', PLAN_AUTHORIZATION_CHANGED: '权限已变化，请重新拟定。', PLAN_TEMPLATE_CHANGED: '模板已变化，请重新拟定。', PLAN_MEMBER_UNAVAILABLE: '计划中的人员已停用，请重新拟定。' };
export function ConversationPlans({ conversation: c, me, onReference, referenceDisabled }: { conversation: Schemas['Conversation']; me: Me; onReference?(plan:Schemas['AgentPlan']):void; referenceDisabled?:boolean }) {
  const [creating,setCreating] = useState(false);
  const enabled=me.actions.includes('agent.plan.read')&&c.type==='private'&&!c.project_id&&c.creator_id===me.user.id;
  const query = useQuery({ queryKey: ['conversation-plans',c.id,c.current_revision_id,me.authorization_version], enabled, queryFn: ({signal}) => api<Schemas['AgentPlanList']>('/conversations/'+c.id+'/plans',{signal}), refetchInterval:5000,gcTime:0 });
  if(!enabled) return null;
  return <section className="panel conversation-summary"><h2>项目创建计划</h2><p className="form-note">核对固定内容后由本人批准。批准需要最近10分钟内完成两步验证；创建成功后会显示项目和业务回执。</p>
    <ErrorNotice error={query.error}/>{query.isPending?<Busy/>:!query.error&&<>{query.data?.items.map(p=><ProjectPlan key={p.id+':'+p.version} plan={p} me={me} onRefresh={()=>query.refetch()} onReference={onReference} referenceDisabled={referenceDisabled}/>)}{!query.data?.items.length&&<Empty title="尚无项目计划" detail="可以由秘书拟定，也可以先填写项目资料并核对批准流程。"/>}{query.data?.has_more&&<p>当前显示最近50份计划。</p>}</>}
    {c.type==='private'&&c.state==='active'&&me.actions.includes('agent.plan.propose')&&me.actions.includes('agent.run')&&<button className="button secondary" onClick={()=>setCreating(v=>!v)}>{creating?'收起填写':'拟定项目计划'}</button>}
    {creating&&<CreateProjectPlan conversation={c} me={me} onDone={()=>{setCreating(false);void query.refetch();}}/>}
    <button className="text-button" onClick={()=>void query.refetch()}>重新核对计划状态</button>
  </section>;
}
function CreateProjectPlan({conversation:c,me,onDone}:{conversation:Schemas['Conversation'];me:Me;onDone():void}) {
  const [name,setName]=useState(''),[scope,setScope]=useState(''),[classification,setClassification]=useState<'internal'|'project_restricted'>('internal'),[reason,setReason]=useState(''),[blocked,setBlocked]=useState(false);
  const [choice,setChoice]=useState(emptyProjectSelection);
  const intent=useRef<{body:Schemas['AgentProjectPlanCreate'];key:string}>();
  const create=useMutation({mutationFn:()=>{
    intent.current??={key:crypto.randomUUID(),body:{expected_version:c.version,context_security_version:c.context_security_version,reason,project:projectPlanProject(me.user.id,name,scope,classification,choice)}};
    return api<Schemas['AgentPlan']>('/conversations/'+c.id+'/plans',{method:'POST',...intent.current}).then(plan=>{if(!projectPlanMatches(plan,intent.current!.body)||plan.conversation_id!==c.id||plan.actor_id!==me.user.id||plan.state!=='proposed')throw new Error('项目计划回执与所选内容不一致，请重新核对。');return plan;});
  },onSuccess:onDone,onError:()=>setBlocked(true),retry:false});
  return <form onSubmit={e=>{e.preventDefault();create.mutate();}}><ErrorNotice error={create.error}/>
    <Field label="项目名称"><input required maxLength={200} value={name} disabled={!!intent.current||create.isPending} onChange={e=>setName(e.target.value)}/></Field>
    <Field label="项目范围"><textarea maxLength={20000} value={scope} disabled={!!intent.current||create.isPending} onChange={e=>setScope(e.target.value)}/></Field>
    <Field label="资料范围"><select value={classification} disabled={!!intent.current||create.isPending} onChange={e=>setClassification(e.target.value as typeof classification)}><option value="internal">公司内部</option><option value="project_restricted">项目受限</option></select></Field>
    <ProjectCatalogSelection conversation={c} me={me} value={choice} onChange={setChoice} disabled={!!intent.current||create.isPending}/><Field label="拟定原因"><input required maxLength={200} value={reason} disabled={!!intent.current||create.isPending} onChange={e=>setReason(e.target.value)}/></Field>
    {blocked&&<p className="notice neutral">请求内容已保留。先重新核对计划状态，再使用原请求核对回执。</p>}
    <button className="button primary" disabled={create.isPending||!name.trim()||!reason.trim()}>{create.isPending?'正在拟定':blocked?'以原请求核对计划回执':'保存待批准计划'}</button>
  </form>;
}
function ProjectPlan({plan:p,me,onRefresh,onReference,referenceDisabled}:{plan:Schemas['AgentPlan'];me:Me;onRefresh():Promise<unknown>;onReference?(plan:Schemas['AgentPlan']):void;referenceDisabled?:boolean}) {
  const cache=useQueryClient();const [reason,setReason]=useState(''),[blocked,setBlocked]=useState(false),[fresh,setFresh]=useState(false);
  const intent=useRef<{action:string;body:Schemas['AgentPlanAction'];key:string}>();
  const change=useMutation({mutationFn:async(action:'approve'|'commit'|'revoke')=>{
    if(intent.current&&intent.current.action!==action) throw new Error('请先重新读取并核对上一次操作结果。');
    intent.current??={action,key:crypto.randomUUID(),body:{expected_version:p.version,plan_hash:p.plan_hash,reason}};
    const result=await api<Schemas['AgentPlan']>('/plans/'+p.id+'/'+action,{method:'POST',body:intent.current.body,key:intent.current.key});
    if(result.id!==p.id||result.plan_hash!==p.plan_hash||(action==='commit'&&(!result.operation_id||!result.project_id||result.state!=='committed'))) throw new Error('计划回执尚未确认，请重新读取。');
  },onSuccess:async()=>{await onRefresh();await cache.invalidateQueries({queryKey:['projects']});},onError:()=>setBlocked(true),retry:false});
  const disabled=change.isPending||!reason.trim();
  return <article className="conversation-source-preview"><h3>{p.project.name} · {states[p.state]}</h3><p>{p.project.scope||'未填写范围说明'}</p><p>负责人：{p.project.owner_id===me.user.id?me.user.display_name:p.project.owner_id} · {p.project.classification==='internal'?'公司内部':'项目受限'}</p>
    <p>项目类型：{p.project.type??'general'}</p><p>额外成员：{p.project.member_ids?.length?p.project.member_ids.join('、'):'未额外指定成员'}</p>
    <p>开始日期：{p.project.start_date??'未设置'} · 截止日期：{p.project.due_date??'未设置'}</p>
    {p.template_snapshot.id?<p>固定模板：{p.template_snapshot.id} · 修订 {p.template_snapshot.published_revision}<code className="full-digest">{p.template_snapshot.schema_sha256}</code></p>:<p>项目模板：未指定；使用标准目录，不生成模板任务。</p>}
    <p>计划标识：<code>{p.id}</code></p>{onReference&&<button className="button secondary" disabled={referenceDisabled} onClick={()=>onReference(p)}>引用这份计划</button>}<p>拟定原因：{p.reason} · 有效期至 {new Date(p.expires_at).toLocaleString('zh-HK',{timeZone:'Asia/Hong_Kong'})}（香港）</p><details><summary>核对准确计划标识</summary><code className="full-digest">{p.plan_hash}</code><p>{p.id} · 版本 {p.version}</p></details>
    {p.blocked_reason&&<p className="notice neutral">{reasons[p.blocked_reason]??'当前权限或依赖资料未通过核对，请重新拟定。'}</p>}
    {p.state==='committed'&&p.project_id&&<p><Link to={'/projects/'+p.project_id}>打开已创建项目</Link> · 业务回执 <code>{p.operation_id}</code></p>}
    {(p.can_approve||p.can_commit||p.can_revoke)&&<><ErrorNotice error={change.error}/><Field label="本次操作说明"><input required maxLength={200} value={reason} disabled={!!intent.current||change.isPending} onChange={e=>setReason(e.target.value)}/></Field>
      {(p.can_approve||p.can_commit)&&<MFAConfirmation onFresh={setFresh}/>}
      {blocked?<><p>操作结果待核对，请重新读取计划；重试将使用相同内容和请求标识。</p><button className="button secondary" disabled={change.isPending||((intent.current?.action==='approve'||intent.current?.action==='commit')&&!fresh)} onClick={()=>change.mutate(intent.current!.action as 'approve'|'commit'|'revoke')}>以原请求核对回执</button></>:<div className="actions">{p.can_approve&&<button className="button primary" disabled={disabled||!fresh} onClick={()=>change.mutate('approve')}>批准这份准确计划</button>}{p.can_commit&&<button className="button primary" disabled={disabled||!fresh} onClick={()=>change.mutate('commit')}>按已批准计划创建项目</button>}{p.can_revoke&&<button className="button secondary" disabled={disabled} onClick={()=>change.mutate('revoke')}>撤销计划</button>}</div>}
    </>}
  </article>;
}
