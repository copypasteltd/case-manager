import { useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery } from '@tanstack/react-query';
import { checkedTelegramDetails, checkedTelegramPolicy, checkedTelegramPolicyReceipt, checkedTelegramReceipt, checkedTelegramReconcileReceipt, checkedTelegramSettings, telegramDeliveryStates, telegramPolicyInput, telegramReconcileInput, type TelegramDetails, type TelegramPolicy, type TelegramPolicyInput, type TelegramReceipt, type TelegramReconcile, type TelegramSetting, type TelegramSettings } from '@copypaste/contracts/notification-telegram';
import { notificationCategories } from '@copypaste/contracts/notifications';
import { api, ApiError, type Me, type Page } from './api';
import { Busy, date, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

export default function NotificationTelegram({ me }: { me: Me }) {
  const [epoch,setEpoch]=useState(0);
  return <TelegramControl key={[me.user.id,me.authorization_version,epoch].join(':')} me={me} done={()=>setEpoch(x=>x+1)}/>;
}
function TelegramControl({me,done}:{me:Me;done():void}) {
  const [editing,setEditing]=useState<string>(),[selected,setSelected]=useState<string>();
  const settings=useQuery({queryKey:['notification-telegram-settings',me.user.id,me.authorization_version],queryFn:async({signal})=>checkedTelegramSettings(await api<TelegramSettings>('/notification-telegram-settings',{signal})),gcTime:0});
  const receipts=useInfiniteQuery({queryKey:['notification-telegram-receipts',me.user.id,me.authorization_version],initialPageParam:'',queryFn:async({pageParam,signal})=>{const p=await api<Page<TelegramReceipt>>('/notification-telegram-deliveries?limit=10'+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal});p.items.forEach(checkedTelegramReceipt);return p;},getNextPageParam:p=>p.next_cursor??undefined,gcTime:0});
  async function refresh(){setEditing(undefined);setSelected(undefined);await Promise.all([settings.refetch(),receipts.refetch()]);done();}
  return <section className="panel conversation-summary" aria-label="TG 私聊业务提醒">
    <div className="section-heading"><h2>TG 私聊业务提醒</h2><button className="button secondary" disabled={settings.isFetching||receipts.isFetching} onClick={()=>void refresh()}>刷新 TG 提醒</button></div>
    <p className="form-note">主动开启后，只向本人已确认绑定的 TG 私聊发送登录提示。合同名称、金额、文件和审批正文仍在系统内查看；站内通知继续保留。每次设置仅处理发布后的新通知，已读、已过期或已失效的事项会停止外发。提醒最长保留24小时投递窗口。</p>
    <ErrorNotice error={settings.error??receipts.error}/>{settings.isPending?<Busy/>:!settings.error&&settings.data&&<>
      {!settings.data.items.length&&<p>请先在账户设置中完成 TG 双向身份绑定，再选择需要接收的业务提醒。</p>}
      {settings.data.items.map(s=><div className="conversation-event" key={s.bot_id}><strong>Bot {s.bot_id} · {s.effective?'本人提醒已开启':s.current?.enabled?'绑定已变更，需重新确认':'本人提醒已关闭'}</strong><p className="form-note">{s.binding_active?'当前绑定有效':'当前没有有效绑定'} · 设置版本 {s.current?.version??0}</p><p>{s.current?.categories.map(c=>notificationCategories[c]).join('、')||'尚未选择通知类型'}</p><button className="button secondary" disabled={Boolean(editing)} onClick={()=>setEditing(s.bot_id)}>修改本人 TG 提醒</button>{editing===s.bot_id&&<PolicyForm setting={s} done={()=>void refresh()} cancel={()=>setEditing(undefined)}/>}<PolicyHistory me={me} bot={s.bot_id}/></div>)}
    </>}
    <h3>业务通知投递回执</h3><p className="form-note">服务商确认送达只代表接口接收；本人核对送达为人工记录，均不代表已读或业务已处理。结果未知时请先查看私聊，明确重试可能重复发送。</p>
    {receipts.isPending?<Busy/>:!receipts.error&&<>{receipts.data?.pages.flatMap(p=>p.items).map(d=><div className="conversation-event" key={d.id}><strong>{telegramDeliveryStates[d.state]}</strong><p>Bot {d.bot_id} · 尝试 {d.attempts}/5 · 版本 {d.version} · {date(d.created_at)}</p><p className="form-note">{d.reason_code}{d.message_id!==null?' · 消息号 '+d.message_id:''}</p><button className="button secondary" onClick={()=>setSelected(d.id)}>查看投递与处置记录</button></div>)}{!receipts.data?.pages.some(p=>p.items.length)&&<p>暂无 TG 业务通知投递记录。</p>}{receipts.hasNextPage&&<button className="button secondary" disabled={receipts.isFetchingNextPage} onClick={()=>void receipts.fetchNextPage()}>加载更早投递</button>}</>}
    {selected&&<ReceiptDetails key={selected} me={me} id={selected} done={()=>void refresh()} close={()=>setSelected(undefined)}/>}
  </section>;
}
function useTelegramCommand<T>(done:()=>void) {
  const [blocked,setBlocked]=useState(false);const pending=useRef<{body:string;key:string}>();
  const command=useMutation({mutationFn:async(args:{path:string;body:unknown;validate(v:T):T})=>{
    const body=JSON.stringify({path:args.path,body:args.body});if(!pending.current||pending.current.body!==body)pending.current={body,key:crypto.randomUUID()};let returned=false;
    try{const value=await api<T>(args.path,{method:'POST',body:args.body,key:pending.current.key});returned=true;return args.validate(value);}
    catch(cause){if(returned||!(cause instanceof ApiError)||cause.status===0||cause.status>=500||[403,404,409,412].includes(cause.status))setBlocked(true);throw cause;}
  },onSuccess:done});
  return {...command,blocked,locked:command.isPending||blocked};
}
function PolicyForm({setting,done,cancel}:{setting:TelegramSetting;done():void;cancel():void}) {
  const [enabled,setEnabled]=useState(setting.current?.enabled??false),[categories,setCategories]=useState<string[]>(setting.current?.categories??[]),[reason,setReason]=useState(''),[confirmed,setConfirmed]=useState(false),[fresh,setFresh]=useState(false),[error,setError]=useState<unknown>();
  const command=useTelegramCommand<TelegramPolicy>(done);
  function submit(){setError(undefined);let input:TelegramPolicyInput;try{input=telegramPolicyInput(setting,enabled,categories,reason);}catch(e){setError(e);return;}command.mutate({path:'/notification-telegram-policies/'+setting.bot_id,body:input,validate:v=>checkedTelegramPolicyReceipt(v,input,setting.bot_id)});}
  return <form className="inline-editor" aria-label="本人 TG 提醒设置" onSubmit={e=>{e.preventDefault();submit();}}>
    <Field label="私聊提醒状态"><select value={enabled?'enabled':'disabled'} disabled={command.locked} onChange={e=>{setConfirmed(false);setEnabled(e.target.value==='enabled');}}><option value="disabled">关闭</option><option value="enabled" disabled={!setting.binding_active}>开启</option></select></Field>
    <fieldset disabled={command.locked}><legend>接收的通知类型</legend>{Object.entries(notificationCategories).map(([key,label])=><label className="checkbox-row" key={key}><input type="checkbox" checked={categories.includes(key)} onChange={e=>{setConfirmed(false);setCategories(old=>e.target.checked?[...old,key]:old.filter(x=>x!==key));}}/>{label}</label>)}</fieldset>
    <Field label="提醒设置变更原因"><textarea required maxLength={200} value={reason} disabled={command.locked} onChange={e=>{setConfirmed(false);setReason(e.target.value);}}/></Field>
    <label className="checkbox-row"><input type="checkbox" checked={confirmed} disabled={command.locked} onChange={e=>setConfirmed(e.target.checked)}/>我确认只向本人私聊发送提示；新设置不补发旧通知。</label><MFAConfirmation onFresh={setFresh}/><ErrorNotice error={error??command.error}/>{command.blocked&&<p>请刷新设置与发布历史，核对本次结果后再继续。</p>}
    <button className="button primary" disabled={!confirmed||!fresh||!reason.trim()||command.locked}>保存本人 TG 提醒</button><button type="button" className="button secondary" disabled={command.isPending} onClick={cancel}>取消修改</button>
  </form>;
}
function PolicyHistory({me,bot}:{me:Me;bot:string}){
  const q=useInfiniteQuery({queryKey:['notification-telegram-policy-history',me.user.id,me.authorization_version,bot],initialPageParam:'',queryFn:async({pageParam,signal})=>{const p=await api<Page<TelegramPolicy>>('/notification-telegram-policies/'+bot+'?limit=5'+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal});p.items.forEach(checkedTelegramPolicy);return p;},getNextPageParam:p=>p.next_cursor??undefined,gcTime:0});
  return <details><summary>查看本人提醒设置历史</summary><ErrorNotice error={q.error}/>{q.isPending?<Busy/>:!q.error&&<>{q.data?.pages.flatMap(p=>p.items).map(p=><p key={p.id}>版本 {p.version} · {p.enabled?'开启':'关闭'} · {date(p.created_at)}<br/>{p.categories.map(c=>notificationCategories[c]).join('、')||'无类型'} · {p.reason}</p>)}{q.hasNextPage&&<button className="button secondary" disabled={q.isFetchingNextPage} onClick={()=>void q.fetchNextPage()}>加载更早提醒设置</button>}</>}</details>;
}
function ReceiptDetails({me,id,done,close}:{me:Me;id:string;done():void;close():void}){
  const q=useQuery({queryKey:['notification-telegram-receipt',me.user.id,me.authorization_version,id],queryFn:async({signal})=>checkedTelegramDetails(await api<TelegramDetails>('/notification-telegram-deliveries/'+id,{signal})),gcTime:0});
  return <div className="inline-editor" aria-label="TG 投递详情"><h3>投递详情</h3><ErrorNotice error={q.error}/>{q.isPending?<Busy/>:!q.error&&q.data&&<><p>{telegramDeliveryStates[q.data.receipt.state]} · 消息号 {q.data.receipt.message_id??'暂无'}</p>{q.data.events.map(e=><p key={e.id}>版本 {e.version} · {telegramDeliveryStates[e.state]} · {date(e.created_at)} · {e.reason_code}{e.reason?' · '+e.reason:''}</p>)}{['pending','retry_wait','delivery_unknown','failed'].includes(q.data.receipt.state)&&<ReconcileForm receipt={q.data.receipt} done={done}/>}</>}<button className="button secondary" onClick={close}>关闭投递详情</button></div>;
}
function ReconcileForm({receipt,done}:{receipt:TelegramReceipt;done():void}){
  const [action,setAction]=useState<TelegramReconcile['action']>('abandon'),[message,setMessage]=useState(''),[reason,setReason]=useState(''),[confirmed,setConfirmed]=useState(false),[fresh,setFresh]=useState(false),[error,setError]=useState<unknown>();const command=useTelegramCommand<TelegramReceipt>(done);
  function submit(){setError(undefined);let input:TelegramReconcile;try{input=telegramReconcileInput(receipt,action,message,reason);}catch(e){setError(e);return;}command.mutate({path:'/notification-telegram-deliveries/'+receipt.id+'/reconcile',body:input,validate:v=>checkedTelegramReconcileReceipt(v,receipt,input)});}
  return <form aria-label="本人 TG 投递处置" onSubmit={e=>{e.preventDefault();submit();}}><Field label="本次处置"><select value={action} disabled={command.locked} onChange={e=>{setConfirmed(false);setAction(e.target.value as TelegramReconcile['action']);setMessage('');}}><option value="abandon">终止后续投递</option>{receipt.state==='delivery_unknown'&&<option value="confirm_sent">本人核对已送达</option>}{['delivery_unknown','failed'].includes(receipt.state)&&receipt.attempts<5&&<option value="retry">明确重试一次（可能重复）</option>}</select></Field>{action==='confirm_sent'&&<Field label="本人私聊消息号"><input value={message} maxLength={16} disabled={command.locked} onChange={e=>{setConfirmed(false);setMessage(e.target.value);}}/></Field>}<Field label="投递处置原因"><textarea value={reason} required maxLength={200} disabled={command.locked} onChange={e=>{setConfirmed(false);setReason(e.target.value);}}/></Field><label className="checkbox-row"><input type="checkbox" checked={confirmed} disabled={command.locked} onChange={e=>setConfirmed(e.target.checked)}/>我已核对私聊与当前回执；重试可能重复，确认送达不等于已读。</label><MFAConfirmation onFresh={setFresh}/><ErrorNotice error={error??command.error}/>{command.blocked&&<p>请刷新投递详情，核对处置结果后再继续。</p>}<button className="button primary" disabled={!confirmed||!fresh||!reason.trim()||command.locked}>确认投递处置</button></form>;
}
