import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { makeOATextAdopt, oaTextNames, oaTextOriginal, oaTextProven } from '@copypaste/contracts/oa-text';
import { oaKindNames, oaPaths } from '@copypaste/contracts/oa-request';
import { api, type Schemas } from './api';
import { ErrorNotice, Field } from './components';

export function OATextReview({run}:{run:Schemas['Run']}) {
 const cache=useQueryClient(),base=run.oa_request!;
 const [saved,setSaved]=useState<Schemas['AgentOARequestView']>(),[reviewed,setReviewed]=useState(false),[reason,setReason]=useState(''),[busy,setBusy]=useState(false),[blocked,setBlocked]=useState(false),[error,setError]=useState<unknown>();
 const alive=useRef(true),flight=useRef(false),key=useRef(crypto.randomUUID());
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 const v=saved??base,d=v.text_draft,proven=oaTextProven(v),adopted=proven&&v.status==='adopted';
 async function adopt(){
  if(flight.current||blocked)return;
  let body:Schemas['AgentOATextAdopt'];try{body=makeOATextAdopt(v,reviewed,reason);}catch(e){setError(e);return;}
  flight.current=true;setBusy(true);setError(undefined);
  try{
   const result=await api<Schemas['AgentOARequestView']>('/runs/'+run.id+'/oa-text-adoption',{method:'POST',body,key:key.current});
   if(result.run_id!==run.id||result.intent_sha256!==v.intent_sha256||result.text_draft?.draft_sha256!==d?.draft_sha256||result.status!=='adopted'||!oaTextProven(result))throw new Error('保存结果尚未核对，请重新读取任务和业务修订。');
   if(alive.current){setSaved(result);setBlocked(true);setReason('');setReviewed(false);}
   await cache.invalidateQueries({queryKey:['agent-run-events',run.id]});
  }catch(e){if(alive.current){setError(e);setBlocked(true);}}
  finally{flight.current=false;if(alive.current)setBusy(false);}
 }
 return <section className='conversation-source-preview oa-prose-review'><h4>业务正文草稿</h4><p>{oaKindNames[v.kind]} · {v.before.business.title}</p><ErrorNotice error={error}/>
 {!proven?<p>已记录正文起草目标，等待秘书保存完整建议。</p>:<>
 <p className='form-note'>{adopted?'正文已采用并保存为业务新修订。':'逐字段核对原文与建议，确认后保存业务新修订。'}</p>
 {Object.entries(d!.fields).map(([field,text])=><section className='oa-prose-field' key={field}><h5>{oaTextNames[field]}</h5><div className='oa-prose-comparison'><div><strong>原文</strong><p className='oa-prose-text'>{oaTextOriginal(v,field)||'（空白）'}</p></div><div><strong>正文建议</strong><p className='oa-prose-text'>{text}</p></div></div></section>)}
 {adopted?<p>业务修订 {d!.result!.version} · 操作 {d!.operation_id}</p>:run.state==='succeeded'&&run.outcome==='draft'&&!run.cancel_requested_at?<><label><input type='checkbox' checked={reviewed} disabled={busy||blocked} onChange={e=>setReviewed(e.target.checked)}/>我已核对全部正文建议，确认采用</label><Field label='采用正文的依据'><input value={reason} maxLength={200} disabled={busy||blocked} onChange={e=>setReason(e.target.value)}/></Field><button type='button' className='button secondary' disabled={busy||blocked||!reviewed||!reason.trim()} onClick={()=>void adopt()}>{busy?'正在保存正文':'采用正文草稿'}</button>{blocked&&<p className='form-note'>请重新读取任务与业务修订，核对保存结果后继续。</p>}</>:<p>请等待本次任务结束；失败或取消的任务不能采用正文。</p>}
 <p className='form-note'>保存后可另行准备申请和核对送审。</p>
 </>}
 <Link to={oaPaths[v.kind]+'/'+v.business_id}>查看完整业务资料</Link></section>;
}
