import { grantTime } from './telegram-grant-time';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useMutation } from '@tanstack/react-query';
import { grantCancellationAccepted, grantStateLabels, grantToolLabels, untriggeredGrant } from '@copypaste/contracts/telegram-grants';
import { api, type Page, type Schemas } from './api';
import { ErrorNotice, Field } from './components';
import { useCommandKey } from './BusinessSelectors';

export default function TelegramGrantInventory() {
  const [state, setState] = useState('');
  return <div className="telegram-page"><div className="page-heading"><div><span className="eyebrow">WORKSPACE / TELEGRAM</span><h1>我的固定预授权<span className="accent">.</span></h1><p>查询不会触发执行。每份授权只允许一个已核对的准确项目变更。</p></div><Link to="/telegram">返回本人TG入口</Link></div>
    <Field label="预授权状态"><select value={state} onChange={e=>setState(e.target.value)}><option value="">全部状态</option>{Object.entries(grantStateLabels).map(([key,label])=><option key={key} value={key}>{label}</option>)}</select></Field><GrantRows key={state} state={state}/></div>;
}
function GrantRows({state}:{state:string}) {
  const q=useInfiniteQuery({queryKey:['personal-telegram-write-grants',state],initialPageParam:'',queryFn:({pageParam,signal})=>api<Page<Schemas['TelegramWriteGrant']>>('/me/telegram/write-grants?'+new URLSearchParams({limit:'20',...(state?{state}:{}),...(pageParam?{cursor:pageParam}:{})}),{signal}),getNextPageParam:p=>p.next_cursor??undefined,gcTime:0,retry:false});
  const items=q.error?[]:q.data?.pages.flatMap(p=>p.items)??[];
  return <section className="panel"><div className="section-heading"><h2>当前有权查阅的本人预授权</h2><button type="button" className="text-button" disabled={q.isFetching} onClick={()=>void q.refetch()}>刷新预授权清单</button></div><p>业务依据、原登录、权限、群或绑定变化后须重新建立授权。已触发、实际业务结果及群内披露分别核对；取消不会撤销已完成业务。</p><ErrorNotice error={q.error}/>{q.isPending?<p>正在读取预授权…</p>:!q.error&&<>{items.map(v=><article className="panel" key={v.id+':'+v.run_version+':'+v.state}><h3>{grantToolLabels[v.tool]}</h3><p>{grantStateLabels[v.state]} · 执行状态 {v.run_state} · v{v.run_version}</p><p>建立于 {grantTime(v.created_at)} · 触发到期 {grantTime(v.expires_at)}</p>{v.state==='pending_trigger'&&<code className="cp-grant-trigger">{v.trigger}</code>}<p><Link to={'/conversations/'+v.conversation_id+'?run='+v.run_id}>查看准确指令与实际结果</Link></p>{untriggeredGrant(v)&&<CancelGrant value={v} saved={()=>void q.refetch()}/>}</article>)}{items.length===0&&<p>{q.hasNextPage?'本页无当前可见项目，请继续读取剩余清单。':'当前清单没有符合条件的预授权。'}</p>}</>}{q.hasNextPage&&!q.error&&<button type="button" className="button secondary" disabled={q.isFetchingNextPage} onClick={()=>void q.fetchNextPage()}>继续读取更早预授权</button>}</section>;
}
function CancelGrant({value,saved}:{value:Schemas['TelegramWriteGrant'];saved():void}) {
  const [reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false);const key=useCommandKey();
  const command=useMutation({mutationFn:async()=>{if(!reviewed||!reason.trim())throw new Error('请核对准确授权并填写取消原因。');const body={expected_version:value.run_version,action:'cancel',reason};const run=await api<Schemas['Run']>('/runs/'+value.run_id+'/cancel',{method:'POST',body,key:key(body)});if(!grantCancellationAccepted(run,value))throw new Error('取消回执与原任务不一致，请重新查阅实际结果。');return run;},onSuccess:saved,retry:false});
  return <fieldset disabled={command.isPending}><Field label="取消这份预授权的原因"><input maxLength={200} value={reason} onChange={e=>{setReason(e.target.value);setReviewed(false);}}/></Field><label className="check-field"><input type="checkbox" checked={reviewed} onChange={e=>setReviewed(e.target.checked)}/>已核对本份授权，申请停止尚未完成的执行。</label><ErrorNotice error={command.error}/><button type="button" className="button secondary" disabled={!reviewed||!reason.trim()} onClick={()=>command.mutate()}>取消此预授权执行</button></fieldset>;
}
