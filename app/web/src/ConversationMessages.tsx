import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type Page, type Schemas } from './api';
import { Busy, date, Empty, ErrorNotice, Field } from './components';
import { ConversationFinanceReport } from './ConversationFinanceReport';
import { ConversationServiceReport } from './ConversationServiceReport';
import { ConversationSources } from './ConversationSources';
import { ConversationPlans } from './ConversationPlans';
import { ConversationOrganization } from './ConversationOrganization';
import { ConversationMutation, MutationReceipt } from './ConversationMutation';
import { ConversationFileUpdate, FileUpdateReceipt } from './ConversationFileUpdate';
import { oaRequestAccepted, type OARequest } from '@copypaste/contracts/oa-request';
import { postWriteAccepted, type PostWrite } from '@copypaste/contracts/post-write';
import { ConversationPostWrite, PostWriteReceipt } from './ConversationPostWrite';
import { ConversationOARequest, OARequestReceipt } from './ConversationOARequest';
import { ConversationFileCreate } from './ConversationFileCreate';
import { ConversationFileRecovery } from './ConversationFileRecovery';
import { fileRecoveryReceipt, type FileRecovery } from '@copypaste/contracts/file-recovery';
import { fileUpdateReceipt, fileCreateReceipt, type FileUpdate, type FileCreate } from '@copypaste/contracts/file-update';
import { mutationReceipt, type Mutation } from '@copypaste/contracts/mutation';

type SelectedSource=Pick<Schemas['ConversationContextView'],'id'|'title'|'resource_version'|'version_id'>&{type:Schemas['AgentSourceSnapshot']['type']};
const names: Record<Schemas['Run']['state'], string> = { queued: '等待处理', running: '正在处理', awaiting_approval: '等待计划批准', committing: '正在核对正式提交', succeeded: '已完成', failed: '未完成', cancelled: '已取消', interrupted: '已中断，需核对结果' };
function runName(run: Schemas['Run']) {
  if(run.state!=='succeeded') return names[run.state];
  const outcomes:Record<string,string>={draft:'已回复，草稿待处理',pending:'已回复，事项待处理',awaiting_input:'已回复，等待补充资料',completed:'已回复，本次事项已完成',conflict:'已回复，需核对冲突',denied:'已回复，事项权限不足',failed:'已回复，事项未完成'};
  return outcomes[run.outcome??'']??'已回复，请核对事项结果';
}
const skills: Record<string, string> = { 'project-create': '新建项目', 'project-update': '更新项目', 'file-update': '新建或更新文件', 'information-organize': '整理资料', 'oa-assist': '协助业务申请', 'subscription-review': '核对订阅支出', 'project-briefing': '整理项目简报', 'incident-summary': '整理运行事件' };
const reasons: Record<string, string> = { CHANNEL_MANAGED: '此会话由TG入口管理，请从指定群话题发送新的请求。', RUNTIME_UNAVAILABLE: '秘书执行服务暂未启用；已有消息可以继续查阅。', CONTEXT_UNAVAILABLE: '共同阅读范围或 AI 处理许可尚未通过核对。', RUN_NOT_ALLOWED: '当前权限只允许查阅会话。', CONVERSATION_READ_ONLY: '会话已归档，请先恢复再发送。' };
export function ConversationMessages({ conversation: c, me }: { conversation: Schemas['Conversation']; me: Me }) {
  const cache = useQueryClient();
  const [params] = useSearchParams(), linkedRun = params.get('run');
  useEffect(() => { if (linkedRun && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(linkedRun)) setSelected(linkedRun); }, [linkedRun]);
  const pendingKey = ['secretary-intent', me?.user.id, me?.authorization_version, c.id, c.version] as const;
  const pending = useRef(cache.getQueryData<{ payload: Schemas['RunCreate']; key: string }>(pendingKey));
  const [body, setBody] = useState(pending.current?.payload.message ?? ''), [profile, setProfile] = useState(pending.current?.payload.profile_id ?? ''), [refs, setRefs] = useState<SelectedSource[]>([]), [selected, setSelected] = useState<string>(), [blocked, setBlocked] = useState(!!pending.current);
  const identity = useRef({ message: crypto.randomUUID(), key: crypto.randomUUID(), body: '' });
  const [fileUpdate,setFileUpdate]=useState<FileUpdate|undefined>(pending.current?.payload.file_update);
  const [oaRequest,setOARequest]=useState<OARequest|undefined>(pending.current?.payload.oa_request);
  const [postWrite,setPostWrite]=useState<PostWrite|undefined>(pending.current?.payload.post_write);
  const [fileCreate,setFileCreate]=useState<FileCreate|undefined>(pending.current?.payload.file_create);
  const [fileRecovery,setFileRecovery]=useState<FileRecovery|undefined>(pending.current?.payload.file_recovery);
  const [mutation,setMutation]=useState<Mutation|undefined>(pending.current?.payload.mutation);
  const runtime = useQuery({ queryKey: ['conversation-runtime', c.id, c.current_revision_id], queryFn: ({ signal }) => api<Schemas['RuntimeOptions']>('/conversations/' + c.id + '/runtime-options', { signal }), refetchInterval: 5000 });
  const messages = useInfiniteQuery({ queryKey: ['conversation-messages', c.id, c.current_revision_id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['Message']>>('/conversations/' + c.id + '/messages?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 5000, gcTime: 0 });
  const entries = messages.error ? [] : [...new Map(messages.data?.pages.flatMap(page => page.items).map(value => [value.id, value])).values()];
  const refresh = useCallback(() => Promise.all([cache.invalidateQueries({ queryKey: ['conversation-messages', c.id] }), cache.invalidateQueries({ queryKey: ['conversation', c.id] })]), [cache, c.id]);
  const send = useMutation({
    mutationFn: async () => {
      if (blocked) throw new Error('请先核对前一次结果。');
      const payload: Schemas['RunCreate'] = pending.current?.payload ?? { expected_version: c.version, context_security_version: c.context_security_version, profile_id: profile, client_message_id: identity.current.message, message: body, context_refs: refs.map(ref => ({ type: ref.type, id: ref.id, version_id: ref.version_id })), ...(mutation?{mutation}: {}), ...(fileUpdate?{file_update:fileUpdate}:{}), ...(fileCreate?{file_create:fileCreate}:{}), ...(fileRecovery?{file_recovery:fileRecovery}:{}), ...(oaRequest?{oa_request:oaRequest}:{}),...(postWrite?{post_write:postWrite}:{}) };
      if(payload.post_write&&!runtime.data?.profiles.some(v=>v.id===payload.profile_id&&v.skill_id==='project-briefing')) throw new Error('知识操作请交给当前可用的项目简报任务。');
      if(payload.oa_request&&!runtime.data?.profiles.some(value=>value.id===payload.profile_id&&value.skill_id==='oa-assist')) throw new Error('请为准确申请操作选择协助业务申请任务。');
      if((payload.file_update||payload.file_create||payload.file_recovery)&&!runtime.data?.profiles.some(value=>value.id===payload.profile_id&&value.skill_id==='file-update')) throw new Error('准确文件更新请交给当前可用的更新文件任务。');
      if(payload.mutation&&!runtime.data?.profiles.some(value=>value.id===payload.profile_id&&value.skill_id==='project-update')) throw new Error('准确更新请交给当前可用的更新项目任务。');
      if(payload.mutation&&payload.context_refs?.some(ref=>ref.id===payload.mutation!.arguments.id&&ref.type===(payload.mutation!.tool==='update_project'?'project':'task'))) throw new Error('更新对象已包含在准确更新中，请移除重复资料引用。');
      if(payload.context_refs?.some(ref=>ref.type==='agent_organization_plan')&&!runtime.data?.profiles.some(value=>value.id===payload.profile_id&&value.skill_id==='information-organize')) throw new Error('资料整理计划请交给当前可用的整理资料任务核对。');
      if(payload.context_refs?.some(ref=>ref.type==='agent_plan')&&!runtime.data?.profiles.some(value=>value.id===payload.profile_id&&value.skill_id==='project-create')) throw new Error('项目计划请交给当前可用的新建项目任务核对。');
      const canonical = JSON.stringify(payload);
      if (identity.current.body && identity.current.body !== canonical) throw new Error('前次发送尚未核对，不能更换内容重发。');
      identity.current.body = canonical;
      if (!pending.current) {
        if (cache.getQueryCache().findAll({ queryKey: ['secretary-intent'] }).length >= 32) throw new Error('待核对请求已达会话上限，请先处理已有请求。');
        pending.current = { payload, key: identity.current.key };
        cache.setQueryDefaults(pendingKey, { gcTime: Infinity });
        cache.setQueryData(pendingKey, pending.current);
      }
      const result = await api<Schemas['Run']>('/conversations/' + c.id + '/runs', { method: 'POST', body: payload, key: pending.current.key });
      if (result.actor_id !== me?.user.id || result.last_event_seq !== result.version || typeof result.can_cancel !== 'boolean' || result.conversation_id !== c.id || result.profile_id !== payload.profile_id || result.client_message_id !== payload.client_message_id || !Number.isSafeInteger(result.version) || result.version < 1 || !Object.prototype.hasOwnProperty.call(names, result.state)) throw new Error('任务回执不完整，请先核对消息记录。');
      if(!payload.file_recovery&&!fileUpdateReceipt(result,payload.file_update)) throw new Error('准确文件更新回执尚未核对，请先读取任务结果。');
      if(!postWriteAccepted(result,payload.post_write)) throw new Error('知识操作受理记录尚未核对，请先读取任务结果。');
      if(!oaRequestAccepted(result,payload.oa_request)) throw new Error('申请操作受理记录尚未核对，请先读取任务结果。');
      if(!payload.file_recovery&&!fileCreateReceipt(result,payload.file_create)) throw new Error('准确文件新建回执尚未核对，请先读取任务结果。');
      if(!mutationReceipt(result,payload.mutation)) throw new Error('准确更新范围的回执尚未核对，请先读取任务结果。');
      if(!fileRecoveryReceipt(result,payload.file_recovery)) throw new Error('准确草稿恢复回执尚未核对，请先读取任务结果。');
      return result;
    },
    onSuccess: async run => { pending.current = undefined; cache.removeQueries({ queryKey: pendingKey, exact: true }); identity.current = { message: crypto.randomUUID(), key: crypto.randomUUID(), body: '' }; setBody(''); setRefs([]); setMutation(undefined);setOARequest(undefined);setPostWrite(undefined); setFileUpdate(undefined);setFileCreate(undefined);setFileRecovery(undefined); setSelected(run.id); await refresh(); },
    onError: () => setBlocked(true),
    retry: false,
  });
  return <><ConversationOrganization conversation={c} me={me} referenceDisabled={!!pending.current||blocked||send.isPending||refs.length>=20} onReference={runtime.data?.profiles.some(v=>v.skill_id==='information-organize')?plan=>{if(pending.current||blocked||send.isPending||refs.length>=20)return;setRefs(old=>[...old.filter(r=>r.type!=='agent_plan'&&(r.type!=='agent_organization_plan'||r.id!==plan.id)),{type:'agent_organization_plan',id:plan.id,title:'资料整理计划 · '+plan.preview.length+'项',version_id:null,resource_version:1}]);setMutation(undefined);setOARequest(undefined);setPostWrite(undefined);setFileUpdate(undefined);setFileCreate(undefined);setFileRecovery(undefined);setProfile(runtime.data!.profiles.find(v=>v.skill_id==='information-organize')!.id);}:undefined}/><ConversationPlans conversation={c} me={me} referenceDisabled={!!pending.current||blocked||send.isPending||refs.length>=20} onReference={runtime.data?.profiles.some(value=>value.skill_id==='project-create')?plan=>{if(pending.current||blocked||send.isPending||refs.length>=20)return;setRefs(old=>[...old.filter(item=>item.type!=='agent_organization_plan'&&(item.type!=='agent_plan'||item.id!==plan.id)),{type:'agent_plan',id:plan.id,title:plan.project.name,resource_version:1,version_id:null}]);setMutation(undefined);setOARequest(undefined);setPostWrite(undefined);setFileUpdate(undefined);setFileCreate(undefined);setFileRecovery(undefined);setProfile(runtime.data!.profiles.find(value=>value.skill_id==='project-create')!.id);}:undefined}/><ConversationSources conversation={c} onSelect={value => { if (!pending.current && !blocked && !send.isPending && refs.length < 20) setRefs(old => [...old.filter(item => item.type !== value.type || item.id !== value.id || value.type === 'file' && item.version_id !== value.version_id), value]); }} />
    <section className="panel conversation-summary"><h2>给公司秘书的请求</h2><ErrorNotice error={runtime.error} />{runtime.isPending ? <Busy label="正在核对执行服务" /> : runtime.data?.available && !runtime.error ? <form onSubmit={event => { event.preventDefault(); send.mutate(); }}>
      <ErrorNotice error={send.error} /><Field label="本次秘书任务"><select required value={profile} disabled={!!pending.current || blocked || send.isPending} onChange={event => setProfile(event.target.value)}><option value="">请明确选择已配置任务</option>{runtime.data.profiles.filter(value=>(!refs.some(ref=>ref.type==='agent_organization_plan')||value.skill_id==='information-organize')&&(!refs.some(ref=>ref.type==='agent_plan')||value.skill_id==='project-create')&&(!mutation||value.skill_id==='project-update')&&(!(fileUpdate||fileCreate||fileRecovery)||value.skill_id==='file-update')&&(!oaRequest||value.skill_id==='oa-assist')&&(!postWrite||value.skill_id==='project-briefing')).map(value => <option key={value.id} value={value.id}>{skills[value.skill_id] ?? value.skill_id} · {value.skill_version}</option>)}</select></Field>
      {runtime.data.profiles.some(value=>value.id===profile&&value.skill_id==='project-update')&&<ConversationMutation conversation={c} disabled={!!pending.current||blocked||send.isPending} value={mutation} onChange={setMutation}/>}
      {runtime.data.profiles.some(v=>v.id===profile&&v.skill_id==='project-briefing')&&<ConversationPostWrite conversation={c} disabled={!!pending.current||blocked||send.isPending} value={postWrite} references={refs.map(r=>({type:r.type,id:r.id,...(r.version_id?{version_id:r.version_id}:{})}))} onChange={setPostWrite}/>}
      {runtime.data.profiles.some(value=>value.id===profile&&value.skill_id==='oa-assist')&&<ConversationOARequest conversation={c} disabled={!!pending.current||blocked||send.isPending} value={oaRequest} onChange={setOARequest}/>}
      {runtime.data.profiles.some(value=>value.id===profile&&value.skill_id==='file-update')&&<><ConversationFileUpdate conversation={c} disabled={!!pending.current||blocked||send.isPending||!!fileCreate||!!fileRecovery} value={fileUpdate} onChange={setFileUpdate}/><ConversationFileCreate conversation={c} disabled={!!pending.current||blocked||send.isPending||!!fileUpdate||!!fileRecovery} value={fileCreate} onChange={setFileCreate}/><ConversationFileRecovery conversation={c} actor={me.user.id} disabled={!!pending.current||blocked||send.isPending||!!fileUpdate||!!fileCreate} value={fileRecovery} onChange={setFileRecovery}/></>}
      <Field label="本次消息内容"><textarea required rows={5} maxLength={20000} value={body} disabled={!!pending.current || blocked || send.isPending} onChange={event => setBody(event.target.value)} /></Field>
      {refs.map(ref => <p key={ref.type + ref.id + (ref.version_id??'')}>{ref.title} · {(ref.type==='agent_plan'||ref.type==='agent_organization_plan')?'固定计划意图':'固定修订 '+ref.resource_version} <button type="button" className="text-button" disabled={!!pending.current || blocked || send.isPending} onClick={() => setRefs(old => old.filter(item => item !== ref))}>移除此项</button></p>)}
      <p className="form-note">最多20项明确资料。引用计划只提供准确内容，不代表已批准或允许执行。正式写入、批量计划与业务完成均以服务器核对结果为准。</p>
      {blocked ? <div className="notice neutral"><p>输入和本次请求标识仍保留。先读取消息和任务结果；相同请求可明确重试核对，不会自动另建任务。</p><button type="button" className="button secondary" onClick={() => void refresh()}>重新读取消息核对结果</button><button type="button" className="button secondary" onClick={() => { setBlocked(false); }}>允许以原请求继续核对</button></div> : <button className="button primary" disabled={send.isPending || !body.trim() || !runtime.data.profiles.some(value => value.id === profile)}>{send.isPending ? '正在提交' : '确认发送本次请求'}</button>}
    </form> : <p className="notice neutral">{reasons[runtime.data?.reason ?? ''] ?? '当前执行配置尚未确认，请重新读取。'}</p>}</section>
    {selected && <RunDetails key={selected} id={selected} conversationID={c.id} onClose={() => setSelected(undefined)} onCommitted={refresh} />}
    <section className="panel conversation-summary"><h2>实际消息记录</h2><p className="form-note">消息只向发送时的明确读者开放，正文和引用继续检查当前资料权限。</p><ErrorNotice error={messages.error} />{messages.isPending ? <Busy label="正在读取实际消息" /> : entries.length ? entries.map(message => <article className="conversation-source-preview" key={message.id}><h3>{message.role === 'user' ? '成员消息' : message.role === 'assistant' ? '秘书回复' : '系统记录'} · {message.seq}</h3><p className="muted">{date(message.created_at)}</p><pre>{message.visibility === 'available' ? message.body : '依赖资料已变化或当前不可查阅，正文和引用已隐藏。'}</pre>{message.source_refs.map(ref => <div key={ref.type + ref.id + (ref.version_id??'')}>固定资料修订 {ref.resource_version}<code className="full-digest">{ref.sha256}</code>{ref.type==='agent_finance_report'&&<ConversationFinanceReport conversation={c} source={ref}/>}
{ref.type==='service'&&ref.version_id&&<ConversationServiceReport conversation={c} source={ref}/>}</div>)}{message.visibility === 'available' && <button className="button secondary" onClick={() => setSelected(message.run_id)}>查看任务进度</button>}</article>) : !messages.error && <Empty title="尚无已保存消息" detail="只有真正受理的请求才进入消息记录。" />}{messages.hasNextPage && <button className="button secondary" disabled={messages.isFetchingNextPage} onClick={() => void messages.fetchNextPage()}>加载更早消息</button>}</section>
  </>;
}
function RunDetails({ id, conversationID, onClose, onCommitted }: { id: string; conversationID: string; onClose(): void; onCommitted(): Promise<unknown> }) {
  const [reason, setReason] = useState(''), [blocked, setBlocked] = useState(false), [after, setAfter] = useState(0), key = useRef(crypto.randomUUID());
  const query = useQuery({ queryKey: ['agent-run-events', id, after, conversationID], queryFn: async ({ signal }) => { const value = await api<Schemas['RunEventPage']>('/runs/' + id + '/event-log?after_seq=' + after + '&limit=100', { signal }); if (value.run.id !== id || value.run.conversation_id !== conversationID) throw new Error('任务与当前会话不一致，请从通知重新打开。'); return value; }, refetchInterval: 3000, gcTime: 0 });
  useEffect(() => { if (query.error) void onCommitted(); }, [query.error, onCommitted]);
  const run = query.error ? undefined : query.data?.run;
  const cancel = useMutation({ mutationFn: async () => { if (!run) throw new Error('请先读取当前任务。'); const result = await api<Schemas['Run']>('/runs/' + id + '/cancel', { method: 'POST', key: key.current, body: { expected_version: run.version, action: 'cancel', reason } }); if (result.id !== run.id || result.version !== run.version + 1 || !result.cancel_requested_at) throw new Error('取消回执尚未确认，请重新读取任务。'); }, onSuccess: async () => { setReason(''); await query.refetch(); await onCommitted(); }, onError: () => setBlocked(true), retry: false });
  return <section className="panel conversation-summary"><h2>任务当前进展</h2><ErrorNotice error={query.error ?? cancel.error} />{query.isPending ? <Busy label="正在读取任务" /> : run && <><p><strong>{runName(run)}</strong></p>{run.post_write&&<PostWriteReceipt run={run}/>}{run.oa_request&&<OARequestReceipt run={run}/>}{run.mutation&&<MutationReceipt value={run.mutation}/>}{run.file_update&&<FileUpdateReceipt value={run.file_update} stopped={!!run.cancel_requested_at||!['queued','running','awaiting_approval','committing'].includes(run.state)}/>}{run.file_create&&<FileUpdateReceipt value={run.file_create} stopped={!!run.cancel_requested_at||!['queued','running','awaiting_approval','committing'].includes(run.state)}/>}{run.cancel_requested_at && <p className="notice neutral">{run.state === 'cancelled' ? '已确认取消。已提交的历史业务事实仍保留。' : '已请求取消，执行停止尚待核对，请以实际结果为准。'}</p>}{run.state === 'committing' && <p className="notice neutral">正在核对正式提交结果，不能宣称已撤销。</p>}<ol>{query.data!.items.map(event => <li key={event.id}>{event.seq} · {names[event.state]} · {date(event.created_at)}{event.safe_code && <code>{event.safe_code}</code>}</li>)}</ol>{after > 0 && <button className="button secondary" onClick={() => setAfter(Math.max(0, after - 100))}>读取上一批事件</button>}{query.data!.has_more && <button className="button secondary" onClick={() => setAfter(query.data!.after_seq)}>继续读取后续事件</button>}{run.can_cancel && <form onSubmit={event => { event.preventDefault(); cancel.mutate(); }}><Field label="取消任务的原因"><input required maxLength={200} disabled={cancel.isPending || blocked} value={reason} onChange={event => setReason(event.target.value)} /></Field><button className="button secondary" disabled={cancel.isPending || blocked || !reason.trim()}>确认请求取消</button></form>}{blocked && <p className="notice neutral">取消结果需重新核对，未自动重发。请读取实际任务状态后继续。</p>}</>}<button className="button secondary" onClick={() => void query.refetch()}>重新读取任务结果</button><button className="button secondary" onClick={onClose}>收起任务进度</button></section>;
}
