import { Text, View } from '@tarojs/components';
import { useEffect, useRef, useState } from 'react';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { go } from '../../platform/page';
import { Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice, Section, date } from '../../ui';
import { SelectField, TextField } from '../business/controls';

const states = { proposed: '待本人批准', approved: '已批准，等待创建', committed: '项目已创建', revoked: '已撤销' };
export function ConversationPlans({conversation:c,me,onReference,referenceDisabled}:{conversation:Schemas['Conversation'];me:Schemas['Me'];onReference?(plan:Schemas['AgentPlan']):void;referenceDisabled?:boolean}) {
  const [value,setValue]=useState<Schemas['AgentPlanList']>(),[error,setError]=useState<unknown>(),[loading,setLoading]=useState(true),[revision,setRevision]=useState(0),[creating,setCreating]=useState(false),[verifiedAt,setVerifiedAt]=useState<string|null>(null);
  const enabled=me.actions.includes('agent.plan.read')&&c.type==='private'&&!c.project_id&&c.creator_id===me.user.id;
  useEffect(()=>{
    const scope=new Scope();let flight=false;setValue(undefined);setError(undefined);setLoading(enabled);
    async function read(){if(scope.closed||flight||!enabled)return;flight=true;
      try{const [result,currentMe]=await Promise.all([client.request<Schemas['AgentPlanList']>('/conversations/'+c.id+'/plans',{scope}),client.request<Schemas['Me']>('/me',{scope})]);if(!scope.closed){setValue(result);setVerifiedAt(currentMe.mfa_verified_at);setError(undefined);}}
      catch(cause){if(!scope.closed){setValue(undefined);setError(cause);}}
      finally{flight=false;if(!scope.closed)setLoading(false);}
    }
    void read();const timer=setInterval(()=>void read(),5000);return()=>{clearInterval(timer);scope.close();};
  },[c.id,c.current_revision_id,me.authorization_version,enabled,revision]);
  if(!enabled)return null;
  const refresh=()=>setRevision(v=>v+1);
  return <Section title='项目创建计划'><View className='cp-pad'><Text className='cp-paragraph'>本人核对并批准固定内容后，再创建项目。批准与创建均需要最近10分钟内完成两步验证。</Text><Notice error={error}/>
    {loading?<Busy/>:value&&<>{value.items.map(p=><PlanCard key={p.id+':'+p.version} plan={p} me={me} recentMFA={!!verifiedAt&&Date.parse(verifiedAt)<=Date.now()+5000&&Date.now()-Date.parse(verifiedAt)<9*60000} onDone={refresh} onReference={onReference} referenceDisabled={referenceDisabled}/>)}{!value.items.length&&<Empty>尚无项目计划。可以由秘书拟定，也可以先填写资料。</Empty>}{value.has_more&&<Text>当前显示最近50份计划。</Text>}</>}
    {c.type==='private'&&c.state==='active'&&me.actions.includes('agent.plan.propose')&&me.actions.includes('agent.run')&&<Action secondary onClick={()=>setCreating(v=>!v)}>{creating?'收起填写':'拟定项目计划'}</Action>}
    {creating&&<CreatePlan conversation={c} me={me} onDone={()=>{setCreating(false);refresh();}}/>}<Action secondary onClick={refresh}>重新核对计划状态</Action>
  </View></Section>;
}
function CreatePlan({conversation:c,me,onDone}:{conversation:Schemas['Conversation'];me:Schemas['Me'];onDone():void}){
  const draft=useFormDraft(me,{kind:'agent-project-plan',id:c.id,version:c.version},'propose',{name:'',body:'',action:'internal'});
  const intent=useRef<Schemas['AgentProjectPlanCreate']>();
  const command=useCommand<Schemas['AgentPlan']>(()=>{draft.forget();onDone();},draft.binding,v=>v.conversation_id===c.id&&v.state==='proposed'&&v.actor_id===me.user.id&&v.project.name===intent.current?.project.name&&v.plan_hash.length===64);
  const disabled=command.busy||command.blocked;
  return <View><Notice error={command.error}/><CommandNotice blocked={command.blocked}/><TextField name='plan-name' label='项目名称' value={draft.fields.name??''} onChange={name=>draft.update({name})} disabled={disabled}/>
    <TextField name='plan-scope' label='项目范围' value={draft.fields.body??''} onChange={body=>draft.update({body})} multiline max={20000} disabled={disabled}/>
    <SelectField label='资料范围' value={draft.fields.action??'internal'} options={[[ 'internal','公司内部' ],[ 'project_restricted','项目受限' ]]} onChange={action=>draft.update({action})} disabled={disabled}/>
    <Fact label='项目负责人'>{me.user.display_name}（本人）</Fact><TextField name='plan-reason' label='拟定原因' value={draft.fields.reason} onChange={reason=>draft.update({reason})} disabled={disabled}/>
    <Action loading={command.busy} disabled={disabled||!draft.fields.name?.trim()||!draft.fields.reason.trim()} onClick={()=>{
      const body:Schemas['AgentProjectPlanCreate']={expected_version:c.version,context_security_version:c.context_security_version,reason:draft.fields.reason,project:{name:draft.fields.name!,scope:draft.fields.body??'',owner_id:me.user.id,classification:draft.fields.action==='project_restricted'?'project_restricted':'internal'}};
      intent.current=body;void command.submit('/conversations/'+c.id+'/plans',body);
    }}>保存待批准计划</Action>
  </View>;
}
function PlanCard({plan:p,me,recentMFA,onDone,onReference,referenceDisabled,readOnly}:{plan:Schemas['AgentPlan'];me:Schemas['Me'];recentMFA:boolean;onDone():void;onReference?(plan:Schemas['AgentPlan']):void;referenceDisabled?:boolean;readOnly?:boolean}){
  const [fresh,setFresh]=useState(false),intent=useRef<string>();
  const draft=useFormDraft(me,{kind:'agent-plan-action',id:p.id,version:p.version},p.state);
  const command=useCommand<Schemas['AgentPlan']>(()=>{draft.forget();onDone();},draft.binding,v=>v.id===p.id&&v.plan_hash===p.plan_hash&&(intent.current!=='commit'||(v.state==='committed'&&!!v.project_id&&!!v.operation_id)));
  const disabled=command.busy||command.blocked;
  function act(action:'approve'|'commit'|'revoke'){intent.current=action;void command.submit('/plans/'+p.id+'/'+action,{expected_version:p.version,plan_hash:p.plan_hash,reason:draft.fields.reason});}
  return <View className='cp-inset'><Text className='cp-section-title'>{p.project.name} · {states[p.state]}</Text><Text className='cp-paragraph'>{p.project.scope||'未填写范围说明'}</Text>
    <Fact label='负责人'>{p.project.owner_id===me.user.id?me.user.display_name:p.project.owner_id}</Fact><Fact label='资料范围'>{p.project.classification==='internal'?'公司内部':'项目受限'}</Fact>
    <Fact label='项目类型'>{p.project.type??'general'}</Fact><Fact label='开始日期'>{p.project.start_date??'未设置'}</Fact><Fact label='截止日期'>{p.project.due_date??'未设置'}</Fact><Fact label='额外成员'>{p.project.member_ids?.length?p.project.member_ids.join('、'):'未额外指定成员'}</Fact>
    <Fact label='计划标识'><Text selectable className='cp-code'>{p.id}</Text></Fact>{onReference&&<Action secondary disabled={referenceDisabled} onClick={()=>onReference(p)}>引用这份计划</Action>}<Fact label='拟定原因'>{p.reason}</Fact><Fact label='有效期至'>{date(p.expires_at)}</Fact><Fact label='准确计划摘要'>{p.plan_hash}</Fact>
    {p.template_snapshot.id?<><Fact label='固定模板'>{p.template_snapshot.id}</Fact><Fact label='固定模板修订'>{p.template_snapshot.published_revision} · {p.template_snapshot.schema_sha256}</Fact></>:<Fact label='项目模板'>未指定；使用标准目录，不生成模板任务。</Fact>}
    {p.blocked_reason&&<Text className='cp-notice'>计划已过期，或会话、权限、模板、人员发生变化。请重新拟定；仍可撤销当前计划。</Text>}
    {p.state==='committed'&&p.project_id&&<><Fact label='业务回执'>{p.operation_id}</Fact><Action secondary onClick={()=>go('project',p.project_id!)}>打开已创建项目</Action></>}
    {!readOnly&&(p.can_approve||p.can_commit||p.can_revoke)&&<><Notice error={command.error}/><CommandNotice blocked={command.blocked}/>
      <TextField name='action-reason' label='本次操作说明' value={draft.fields.reason} onChange={reason=>draft.update({reason})} disabled={disabled}/>
      {(p.can_approve||p.can_commit)&&!recentMFA&&<MFAConfirmation onFresh={setFresh} disabled={disabled}/>}
      {(p.can_approve||p.can_commit)&&recentMFA&&<Text className='cp-muted cp-small'>最近已完成两步验证，请核对内容后确认操作。</Text>}
      {p.can_approve&&<Action loading={command.busy} disabled={disabled||(!fresh&&!recentMFA)||!draft.fields.reason.trim()} onClick={()=>act('approve')}>批准这份准确计划</Action>}
      {p.can_commit&&<Action loading={command.busy} disabled={disabled||(!fresh&&!recentMFA)||!draft.fields.reason.trim()} onClick={()=>act('commit')}>按已批准计划创建项目</Action>}
      {p.can_revoke&&<Action secondary loading={command.busy} disabled={disabled||!draft.fields.reason.trim()} onClick={()=>act('revoke')}>撤销计划</Action>}
    </>}
  </View>;
}

export function ReferencedPlan({reference,conversation:c,me,onClose}:{reference:Schemas['AgentSourceSnapshot'];conversation:Schemas['Conversation'];me:Schemas['Me'];onClose():void}) {
  const [plan,setPlan]=useState<Schemas['AgentPlan']>(),[error,setError]=useState<unknown>();
  useEffect(()=>{const scope=new Scope();setPlan(undefined);setError(undefined);void client.request<Schemas['AgentPlan']>('/plans/'+reference.id,{scope}).then(value=>{
    if(!scope.closed){if(value.id!==reference.id||value.plan_hash!==reference.sha256||value.conversation_id!==c.id||value.actor_id!==me.user.id){setError(new Error('计划引用不属于当前会话，请重新核对。'));return;}setPlan(value);}
  }).catch(cause=>{if(!scope.closed)setError(cause);});return()=>scope.close();},[reference.id,reference.sha256,c.id,me.authorization_version]);
  return <Section title='历史引用的当前计划'><View className='cp-pad'><Text className='cp-paragraph'>历史引用固定计划内容；以下展示本次读取时的状态，不能代替批准或执行结果。</Text><Notice error={error}/>{plan?<PlanCard plan={plan} me={me} recentMFA={false} onDone={()=>{}} readOnly/>:!error&&<Busy/>}<Action secondary onClick={onClose}>收起当前计划</Action></View></Section>;
}
