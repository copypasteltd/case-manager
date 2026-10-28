import { makeOAText, oaTextFields, oaTextNames } from '@copypaste/contracts/oa-text';
import { OATextReview } from './oa-text';
import { Text, View } from '@tarojs/components';
import { useEffect, useState } from 'react';
import { makeOADraft, makeOASubmit, oaKindNames, oaPaths, oaRequestReceipt, type OABusiness, type OAKind, type OARequest } from '@copypaste/contracts/oa-request';
import { useRecord } from '../../platform/collection';
import { type Schemas } from '../../platform/session';
import { go } from '../../platform/page';
import { Action, Busy, Check, Notice } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';
import { PayloadSnapshot } from '../detail/snapshot';

export function ConversationOARequest({conversation:c,me,disabled,value,onChange}:{conversation:Schemas['Conversation'];me:Schemas['Me'];disabled:boolean;value?:OARequest;onChange(value?:OARequest):void}) {
 const [open,setOpen]=useState(false),[picking,setPicking]=useState(false),[kind,setKind]=useState<OAKind>('contract'),[id,setID]=useState(''),[mode,setMode]=useState<'draft'|'submit'|'text'>('draft'),[fields,setFields]=useState<string[]>(['notes']),[reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false),[error,setError]=useState<unknown>();
 const current=useRecord<OABusiness>(open&&id?oaPaths[kind]+'/'+id:null), b=current.error?undefined:current.value;
 const request=useRecord<Schemas['Request']>(open&&b?.request_id?'/requests/'+b.request_id:null), r=request.error?undefined:request.value;
 const preview=useRecord<Schemas['SubmissionPreview']>(open&&mode==='submit'&&r?.can_submit?'/requests/'+r.id+'/submission-preview':null),p=preview.error?undefined:preview.value;
 useEffect(()=>{setReviewed(false);setError(undefined);},[id,kind,mode,b?.version,r?.version,p?.preview_hash]);
 if(c.type!=='private')return <Text className='cp-paragraph'>申请操作请进入本人私密会话。</Text>;
 if(picking)return <ChoiceList<OABusiness> title='选择申请业务资料' path={oaPaths[kind]+'?limit=20'+(c.project_id?'&project_id='+c.project_id:'')} searchable={false} label={v=>v.title} eligible={v=>v.project_id===c.project_id} onSelect={v=>{if(!disabled){setID(v.id);setPicking(false);}}} onClose={()=>setPicking(false)}/>;
 const waiting=current.loading||request.loading||preview.loading;
 return <View className='cp-editor-row'><Text className='cp-row-title'>本次准确业务申请</Text>{value?<><Text className='cp-paragraph'>{value.tool==='draft_business_text'?'起草选定业务的正文建议':value.tool==='submit_request'?'送审本人确认的申请版本':'起草或修订指定业务的申请'}</Text><Text className='cp-muted cp-paragraph'>已固定本次操作；发送时仍会核对权限、版本与审批预览。</Text><Action secondary disabled={disabled} onClick={()=>{onChange(undefined);setOpen(false);}}>移除本次申请操作</Action></>:!open?<Action secondary disabled={disabled} onClick={()=>setOpen(true)}>选择业务申请操作</Action>:<>
 <Notice error={error??current.error??request.error??preview.error}/>
 <SelectField label='申请业务类型' value={kind} options={Object.entries(oaKindNames) as [string,string][]} disabled={disabled} onChange={v=>{setKind(v as OAKind);setID('');setFields(['notes']);}}/>
 <Action secondary disabled={disabled} onClick={()=>setPicking(true)}>选择申请业务资料</Action>
 <SelectField label='申请操作' value={mode} options={[[ 'text','起草业务正文' ],[ 'draft','起草或修订申请' ],[ 'submit','核对预览后送审' ]]} disabled={disabled} onChange={v=>setMode(v as typeof mode)}/>
 {waiting&&<Busy/>}{b&&<><Text className='cp-paragraph'>{b.title} · 业务修订 {b.version}</Text><Text className='cp-code' selectable>{b.content_hash}</Text><Action secondary onClick={()=>go(kind,b.id)}>查看完整业务资料</Action>{r&&<Text className='cp-paragraph'>关联申请：修订 {r.version} · {r.state}</Text>}</>}
 {mode==='submit'&&p&&<><PayloadSnapshot snapshot={p.payload} digest={p.content_hash} me={me} heading='本次送审预览'/><Text className='cp-paragraph'>审批规则：{p.workflow.name} · 修订 {p.workflow.version}</Text>{p.workflow.steps.map(s=><Text className='cp-paragraph' key={s.seq}>{s.seq} · {s.title} · {s.assigned_user_name}{s.requires_mfa?' · 需要两步验证':''}</Text>)}<Check checked={reviewed} disabled={disabled} onChange={setReviewed}>我已核对业务内容、审批规则和审批人</Check></>}
 {mode==='text'&&<View className='cp-editor-row'><Text className='cp-row-title'>起草字段</Text>{oaTextFields(kind).map(field=><Check key={field} checked={fields.includes(field)} disabled={disabled} onChange={checked=>setFields(old=>checked?[...old,field]:old.filter(v=>v!==field))}>{oaTextNames[field]}</Check>)}<Text className='cp-muted cp-paragraph'>秘书保存正文建议，待本人核对采用后再更新业务。</Text></View>}
 <TextField name='oa-reason' label='申请操作说明' value={reason} max={200} disabled={disabled} onChange={setReason}/>
 <Text className='cp-muted cp-paragraph'>起草使用已保存的业务修订；送审后进入员工审批待办。</Text>
 <Action secondary disabled={disabled||waiting||!b||!reason.trim()||(mode==='submit'&&(!p||!reviewed))} onClick={()=>{try{if(!b)throw new Error('请先选择业务');onChange(mode==='text'?makeOAText(kind,b,c.project_id,fields,reason):mode==='draft'?makeOADraft(kind,b,r,c.project_id,reason):makeOASubmit(kind,b,r!,p!,c.project_id,reason));setOpen(false);}catch(e){setError(e);}}}>将准确申请操作加入请求</Action>
 <Action secondary disabled={disabled} onClick={()=>setOpen(false)}>收起申请选择</Action>
 </>}</View>;
}
export function OARequestReceipt({run}:{run:Schemas['Run']}) {
 if(run.oa_request?.tool==='draft_business_text')return <OATextReview key={run.id} run={run}/>;
 const value=run.oa_request!;
 const proven=oaRequestReceipt(run);
 return <View className='cp-editor-row'><Text className='cp-row-title'>业务申请执行记录</Text><Text className='cp-paragraph'>{oaKindNames[value.kind]} · {value.before.business.title}</Text>{proven&&value.result?<><Text className='cp-paragraph'>{value.tool==='submit_request'?'已送审，等待员工审批':'申请草稿已保存'}</Text><Text className='cp-code' selectable>申请修订 {value.result.version} · 操作 {value.operation_id}</Text><Action secondary onClick={()=>go('request',value.result!.id)}>查看申请与真实审批进度</Action></>:<Text className='cp-paragraph'>已记录本次授权，请以正式申请和操作记录核对执行结果。</Text>}</View>;
}
