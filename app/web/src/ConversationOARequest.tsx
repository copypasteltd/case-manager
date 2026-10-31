import { makeOAText, oaTextFields, oaTextName } from '@copypaste/contracts/oa-text';
import { OATextReview } from './ConversationOAText';
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { makeOADraft, makeOASubmit, oaKindNames, oaPaths, oaRequestReceipt, type OABusiness, type OAKind, type OARequest } from '@copypaste/contracts/oa-request';
import { api, type Page, type Schemas } from './api';
import { Busy, ErrorNotice, Field } from './components';
import { SnapshotFacts } from './Requests';

export function ConversationOARequest({conversation:c,disabled,value,onChange}:{conversation:Schemas['Conversation'];disabled:boolean;value?:OARequest;onChange(value?:OARequest):void}) {
 const [open,setOpen]=useState(false),[kind,setKind]=useState<OAKind>('contract'),[id,setID]=useState(''),[mode,setMode]=useState<'draft'|'submit'|'text'>('draft'),[fields,setFields]=useState<string[]>(['notes']),[reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false),[error,setError]=useState<unknown>();
 const list=useInfiniteQuery({queryKey:['oa-options',c.id,kind],enabled:open,initialPageParam:'',queryFn:({pageParam,signal})=>api<Page<OABusiness>>(oaPaths[kind]+'?limit=20'+(c.project_id?'&project_id='+c.project_id:'')+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal}),getNextPageParam:p=>p.next_cursor??undefined,gcTime:0});
 const current=useQuery({queryKey:['oa-business',c.id,kind,id],enabled:open&&!!id,queryFn:({signal})=>api<OABusiness>(oaPaths[kind]+'/'+id,{signal}),gcTime:0});
 const b=current.error?undefined:current.data;
 const request=useQuery({queryKey:['oa-request',c.id,b?.request_id],enabled:open&&!!b?.request_id,queryFn:({signal})=>api<Schemas['Request']>('/requests/'+b!.request_id,{signal}),gcTime:0});
 const r=request.error?undefined:request.data;
 const preview=useQuery({queryKey:['oa-preview',c.id,r?.id,r?.version],enabled:open&&mode==='submit'&&!!r?.can_submit,queryFn:({signal})=>api<Schemas['SubmissionPreview']>('/requests/'+r!.id+'/submission-preview',{signal}),gcTime:0,retry:false});
 const p=preview.error?undefined:preview.data;
 useEffect(()=>{setReviewed(false);setError(undefined);},[id,kind,mode,b?.version,r?.version,p?.preview_hash]);
 useEffect(()=>{setFields(['notes']);},[id,kind,b?.version]);
 if(c.type!=='private')return <p className="form-note">申请操作请进入本人私密会话。</p>;
 const waiting=current.isFetching||request.isFetching||preview.isFetching;
 return <section className="conversation-source-preview"><h3>本次准确业务申请</h3>{value?<><p>{value.tool==='draft_business_text'?'起草选定业务的正文建议':value.tool==='submit_request'?'送审本人确认的申请版本':'起草或修订指定业务的申请'}</p><p className="form-note">已固定本次操作；发送时仍会重新核对版本、权限与审批预览。</p><button type="button" className="button secondary" disabled={disabled} onClick={()=>{onChange(undefined);setOpen(false);}}>移除本次申请操作</button></>:!open?<button type="button" className="button secondary" disabled={disabled} onClick={()=>setOpen(true)}>选择业务申请操作</button>:<>
 <ErrorNotice error={error??list.error??current.error??request.error??preview.error}/>
 <Field label="申请业务类型"><select value={kind} disabled={disabled} onChange={e=>{setKind(e.target.value as OAKind);setID('');setFields(['notes']);}}>{Object.entries(oaKindNames).map(([k,n])=><option key={k} value={k}>{n}</option>)}</select></Field>
 <Field label="申请业务资料"><select value={id} disabled={disabled} onChange={e=>setID(e.target.value)}><option value="">请选择已保存业务</option>{list.data?.pages.flatMap(p=>p.items).filter(v=>v.project_id===c.project_id).map(v=><option key={v.id} value={v.id}>{v.title}</option>)}</select></Field>
 {list.hasNextPage&&<button type="button" className="button secondary" disabled={disabled||list.isFetchingNextPage} onClick={()=>void list.fetchNextPage()}>加载更多业务</button>}
 <Field label="申请操作"><select value={mode} disabled={disabled} onChange={e=>setMode(e.target.value as typeof mode)}><option value="text">起草业务正文</option><option value="draft">起草或修订申请</option><option value="submit">核对预览后送审</option></select></Field>
 {waiting&&<Busy label="正在核对业务与审批依据"/>}
 {b&&<><p>{b.title} · 业务修订 {b.version}</p><p className="form-note">内容摘要：{b.content_hash}</p><Link to={oaPaths[kind]+'/'+b.id}>查看完整业务资料</Link>{r&&<p>关联申请：修订 {r.version} · {r.state}</p>}</>}
 {mode==='submit'&&p&&<><h4>本次送审预览</h4><SnapshotFacts value={p.payload}/><p>审批规则：{p.workflow.name} · 修订 {p.workflow.version}</p><ol>{p.workflow.steps.map(s=><li key={s.seq}>{s.title} · {s.assigned_user_name}{s.requires_mfa?' · 需要两步验证':''}</li>)}</ol><p className="form-note">预览摘要：{p.preview_hash}</p><label><input type="checkbox" checked={reviewed} disabled={disabled} onChange={e=>setReviewed(e.target.checked)}/>我已核对业务内容、审批规则和审批人</label></>}
 {mode==='text'&&<fieldset disabled={disabled}><legend>起草字段</legend>{oaTextFields(kind,b).map(field=><label key={field}><input type='checkbox' checked={fields.includes(field)} onChange={e=>setFields(old=>e.target.checked?[...old,field]:old.filter(v=>v!==field))}/>{oaTextName(field,b)}</label>)}<p className='form-note'>可逐项选择明细说明。秘书保存建议，本人核对采用后更新草稿；金额、数量、凭证等保留原值。</p></fieldset>}
 <Field label="申请操作说明"><input maxLength={200} value={reason} disabled={disabled} onChange={e=>setReason(e.target.value)}/></Field>
 <p className="form-note">起草使用已保存的业务修订；送审后进入员工审批待办。</p>
 <button type="button" className="button secondary" disabled={disabled||waiting||!b||!reason.trim()||(mode==='submit'&&(!p||!reviewed))} onClick={()=>{try{if(!b)throw new Error('请先选择业务');onChange(mode==='text'?makeOAText(kind,b,c.project_id,fields,reason):mode==='draft'?makeOADraft(kind,b,r,c.project_id,reason):makeOASubmit(kind,b,r!,p!,c.project_id,reason));setOpen(false);}catch(e){setError(e);}}}>将准确申请操作加入请求</button>
 <button type="button" className="button secondary" disabled={disabled} onClick={()=>setOpen(false)}>收起申请选择</button>
 </>}</section>;
}
export function OARequestReceipt({run}:{run:Schemas['Run']}) {
 if(run.oa_request?.tool==='draft_business_text')return <OATextReview key={run.id} run={run}/>;
 const value=run.oa_request!;
 const proven=oaRequestReceipt(run);
 return <section className="conversation-source-preview"><h4>业务申请执行记录</h4><p>{oaKindNames[value.kind]} · {value.before.business.title}</p>{proven&&value.result?<><p>{value.tool==='submit_request'?'已送审，等待员工审批':'申请草稿已保存'}</p><p>申请修订 {value.result.version} · 操作 {value.operation_id}</p><Link to={'/requests/'+value.result.id}>查看申请与真实审批进度</Link></>:<p>已记录本次授权，请以正式申请和操作记录核对执行结果。</p>}</section>;
}
