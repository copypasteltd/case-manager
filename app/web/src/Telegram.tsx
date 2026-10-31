import { useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type Page, type Schemas } from './api';
import { AccountVerification } from './AccountSecurity';
import { Busy, Dialog, ErrorNotice, Field, date } from './components';
import { TelegramDelivery } from './TelegramDelivery';
import TelegramOperations from './TelegramOperations';

type Challenge = Schemas['TelegramChallenge'];
type Binding = Schemas['TelegramBinding'];
type Channel = Schemas['TelegramChannel'];
type Bot = Schemas['TelegramBot'];
type TelegramRequest = Schemas['TelegramRequest'];
const skillNames = { 'project-briefing': '项目简报', 'incident-summary': '服务事件摘要' };

function RequestCard({ value }: { value: TelegramRequest }) {
  const phases = { queued: '秘书任务已排队', running: '秘书任务执行中', awaiting_approval: '秘书任务待批准', committing: '秘书任务核对结果中', succeeded: '秘书任务已结束', failed: '秘书任务失败', cancelled: '秘书任务已取消', interrupted: '秘书任务已中断' };
  const waiting: Record<string, string> = { TELEGRAM_SKILL_UNAVAILABLE: '此任务尚未接通执行工具；请求保留，等待配置。', RUNTIME_UNAVAILABLE: '等待指定任务的执行配置。', RUN_CAPACITY_EXCEEDED: '等待可用任务名额。', CONVERSATION_RUN_ACTIVE: '等待本话题的上一任务结束。' };
  return <article className="telegram-entry" data-telegram-request-id={value.id} data-run-state={value.run_state ?? ''}>
    <h3>{value.state === 'withdrawn' ? '已撤回' : value.run_state ? phases[value.run_state] : '已接收，等待秘书任务'}</h3>
    <p className="form-note">{date(value.received_at)}{value.requested_skill_id && <> · {skillNames[value.requested_skill_id]}</>}</p><p style={{ whiteSpace: 'pre-wrap' }}>{value.text}</p>
    {value.state === 'withdrawn' ? <p className="form-note">请求的有效期、绑定或入口范围已变化，已停止后续执行；已经结束的任务保留实际记录。</p> : !value.run_id && <p className="form-note">{waiting[value.reason_code] ?? '请求已保存，等待渠道调度。'}</p>}
    {value.run_id && value.conversation_id && <Link className="text-button" to={'/conversations/' + value.conversation_id + '?run=' + value.run_id}>查看此秘书任务</Link>}
    <Link className="button secondary" to={'/telegram?request=' + value.id}>查看此请求及群通知回执</Link>
  </article>;
}

function useWrite<T>(path: string, done: (value: T) => void) {
  const cache = useQueryClient(), intent = useRef<{ body: string; key: string }>();
  return useMutation({ mutationFn: (body: unknown) => {
    const serialized = JSON.stringify(body);
    if (intent.current?.body !== serialized) intent.current = { body: serialized, key: crypto.randomUUID() };
    return api<T>(path, { method: path.startsWith('/telegram/channels/') ? 'PATCH' : 'POST', body, key: intent.current.key });
  }, onSuccess: async value => { await cache.invalidateQueries({ queryKey: ['telegram'] }); done(value); } });
}

export default function Telegram({ me }: { me: Me }) {
  const [search] = useSearchParams();
  if (search.get('view') === 'operations') return me.actions.includes('telegram.operations.read') ? <TelegramOperations me={me} /> : <section className="panel"><h1>组织投递运维</h1><p>当前账号没有组织TG投递查阅权限。</p><Link to="/telegram">返回本人TG入口</Link></section>;
  return <PersonalTelegram me={me} />;
}
function PersonalTelegram({ me }: { me: Me }) {
  const cache = useQueryClient();
  const [search] = useSearchParams(), selected = search.get('request');
  const selectedRequest = useQuery({ queryKey: ['telegram', 'request', selected], queryFn: ({ signal }) => api<TelegramRequest>('/me/telegram/requests/' + encodeURIComponent(selected ?? ''), { signal }), enabled: !!selected });
  const status = useQuery({ queryKey: ['telegram', 'own', me.user.id], queryFn: ({ signal }) => api<Schemas['TelegramStatus']>('/me/telegram', { signal }), refetchInterval: query => query.state.data?.challenges.length ? 3000 : false });
  const requests = useInfiniteQuery({ queryKey: ['telegram', 'requests', me.user.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['TelegramRequest']>>('/me/telegram/requests?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: query => query.state.data?.pages.some(page => page.items.some(item => item.state === 'awaiting_executor' || (item.state === 'dispatched' && ['queued', 'running', 'awaiting_approval', 'committing'].includes(item.run_state ?? '')))) ? 3000 : false });
  const [creating, setCreating] = useState(false), [issued, setIssued] = useState<Challenge>(), [confirming, setConfirming] = useState<Challenge>(), [ending, setEnding] = useState<{ kind: 'challenge'; value: Challenge } | { kind: 'binding'; value: Binding }>();
  const value = status.data, items = requests.data?.pages.flatMap(page => page.items) ?? [];
  return <div className="telegram-page">
    <div className="page-heading"><div><span className="eyebrow">WORKSPACE / TELEGRAM</span><h1>Telegram 接入<span className="accent">.</span></h1><p>绑定本人数字身份，核对指定群与话题的接收范围。</p></div><button className="button primary" disabled={!value?.ingress_configured || !!status.error} onClick={() => setCreating(true)}>发起本人绑定</button></div>
    {me.actions.includes('telegram.operations.read') && <Link className="button secondary" to="/telegram?view=operations">组织TG投递运维</Link>}
    <ErrorNotice error={status.error} />{status.isPending && <Busy label="正在读取Telegram接入状态" />}
    {value && <>
      <div className="notice neutral"><span>{value.ingress_configured ? '本入口可接收明确请求；群通知需单独启用，并只发送阶段与登录链接。' : '尚未配置Bot接收入口，请联系管理员。'}{value.delivery_available ? ' 通知进程近期在线，实际发送结果请核对投递记录。' : ' 当前未确认通知进程在线。'} 办理业务可进入<Link to="/conversations">公司秘书</Link>。</span></div>
      {selected && <><section className="panel"><h2>链接对应的本人请求</h2><ErrorNotice error={selectedRequest.error} />{selectedRequest.isPending ? <Busy /> : selectedRequest.data && <RequestCard value={selectedRequest.data} />}</section>{selectedRequest.data && <TelegramDelivery key={selectedRequest.data.id} requestID={selectedRequest.data.id} />}</>}
      <section className="panel"><div className="section-heading"><h2>本人绑定</h2><button className="button secondary" onClick={() => void cache.invalidateQueries({ queryKey: ['telegram'] })}>重新读取接入状态</button></div>
        {value.bindings.length ? value.bindings.map(binding => <article className="telegram-entry" key={binding.id}><h3>@{value.bots.find(bot => bot.bot_id === binding.bot_id)?.username ?? binding.bot_id}</h3><p>已绑定数字身份：<code>{binding.telegram_user_id}</code></p><p className="form-note">绑定时间：{date(binding.created_at)}</p><button className="button secondary" onClick={() => setEnding({ kind: 'binding', value: binding })}>撤销此绑定</button></article>) : <p>尚无有效绑定。私聊证明后，仍需返回本页核对数字身份并确认。</p>}
        {value.challenges.map(challenge => <article className="telegram-entry" key={challenge.id}><h3>待完成的本人绑定</h3><p>Bot：@{value.bots.find(bot => bot.bot_id === challenge.bot_id)?.username ?? challenge.bot_id} · 到期：{date(challenge.expires_at)}</p>
          {challenge.telegram_user_id ? <><p>私聊证明的数字身份：<code>{challenge.telegram_user_id}</code></p><button className="button primary" onClick={() => setConfirming(challenge)}>核对并确认绑定</button></> : <>
            <p>请在所选Bot的私聊中完成证明。不要把绑定码发送到群里。</p>
            {issued?.id === challenge.id && issued.start_url ? <div data-private-capability className="telegram-binding-secret"><a className="button secondary" href={issued.start_url} target="_blank" rel="noopener noreferrer">打开Bot私聊完成证明</a><p className="form-note">也可在该Bot私聊发送：</p><code>/start {issued.start_token}</code></div> : <p className="form-note">绑定链接只在发起时显示。如已关闭原页面，可取消本次流程后重新发起。</p>}
          </>}
          <button className="button secondary" onClick={() => setEnding({ kind: 'challenge', value: challenge })}>取消此次绑定流程</button>
        </article>)}
      </section>
      <AccountVerification />
      <section className="panel"><div className="section-heading"><h2>本人从TG发出的请求</h2></div><ErrorNotice error={requests.error} />{requests.isPending ? <Busy label="正在读取本人TG请求" /> : items.length ? items.map(item => <RequestCard key={item.id} value={item} />) : !requests.error && <p>尚无已接收的本人请求。</p>}{requests.hasNextPage && <button className="button secondary" disabled={requests.isFetchingNextPage} onClick={() => void requests.fetchNextPage()}>加载更早TG请求</button>}</section>
      {me.actions.includes('policy.manage') && <Channels bots={value.bots} />}
    </>}
    {creating && value && <CreateBinding bots={value.bots} onClose={() => setCreating(false)} onSaved={result => { setIssued(result); setCreating(false); }} />}
    {confirming && <ConfirmBinding value={confirming} onClose={() => setConfirming(undefined)} onSaved={() => { setConfirming(undefined); setIssued(undefined); }} />}
    {ending && <EndBinding target={ending} onClose={() => setEnding(undefined)} onSaved={() => { setEnding(undefined); setIssued(undefined); }} />}
  </div>;
}

function CreateBinding({ bots, onClose, onSaved }: { bots: Bot[]; onClose(): void; onSaved(value: Challenge): void }) {
  const mutation = useWrite<Challenge>('/me/telegram/challenges', onSaved);
  return <Dialog title="发起本人TG绑定" onClose={onClose} busy={mutation.isPending}><form className="telegram-form" onSubmit={event => { event.preventDefault(); const form = new FormData(event.currentTarget); mutation.mutate({ bot_id: String(form.get('bot')), reason: String(form.get('reason')) }); }}><ErrorNotice error={mutation.error} /><Field label="选择Bot"><select name="bot" required defaultValue=""><option value="" disabled>请明确选择Bot</option>{bots.map(bot => <option key={bot.bot_id} value={bot.bot_id}>@{bot.username}</option>)}</select></Field><Field label="绑定原因"><input name="reason" maxLength={200} required /></Field><p className="form-note">需要最近10分钟内完成两步验证。绑定码10分钟内有效，只用于所选Bot私聊；发起新的流程会取消该Bot的旧流程。</p><button className="button primary" disabled={mutation.isPending}>生成本人私聊绑定链接</button></form></Dialog>;
}
function ConfirmBinding({ value, onClose, onSaved }: { value: Challenge; onClose(): void; onSaved(): void }) {
  const mutation = useWrite<Binding>('/me/telegram/challenges/' + value.id + '/confirm', onSaved);
  return <Dialog title="确认本人数字身份" onClose={onClose} busy={mutation.isPending}><form className="telegram-form" onSubmit={event => { event.preventDefault(); mutation.mutate({ expected_version: value.version, telegram_user_id: value.telegram_user_id, reason: String(new FormData(event.currentTarget).get('reason')) }); }}><ErrorNotice error={mutation.error} /><p>本次私聊证明的Telegram数字身份：<code>{value.telegram_user_id}</code></p><Field label="身份核对"><label><input type="checkbox" required />我已核对该数字身份属于本人</label></Field><Field label="确认原因"><input name="reason" maxLength={200} required /></Field><p className="form-note">需在发起流程的原登录中完成近期两步验证后确认。昵称和用户名不作为身份依据。</p><button className="button primary" disabled={mutation.isPending}>确认绑定这个数字身份</button></form></Dialog>;
}
function EndBinding({ target, onClose, onSaved }: { target: { kind: 'challenge'; value: Challenge } | { kind: 'binding'; value: Binding }; onClose(): void; onSaved(): void }) {
  const cancelling = target.kind === 'challenge', path = '/me/telegram/' + (cancelling ? 'challenges/' + target.value.id + '/cancel' : 'bindings/' + target.value.id + '/revoke');
  const mutation = useWrite<Challenge | Binding>(path, onSaved);
  return <Dialog title={cancelling ? '取消绑定流程' : '撤销本人TG绑定'} onClose={onClose} busy={mutation.isPending}><form className="telegram-form" onSubmit={event => { event.preventDefault(); mutation.mutate({ expected_version: target.value.version, reason: String(new FormData(event.currentTarget).get('reason')) }); }}><ErrorNotice error={mutation.error} /><p>{cancelling ? '本次绑定码与私聊证明将失效。' : '该数字身份将停止接收新请求；尚未执行的请求一并撤回。'}</p><Field label="结束原因"><input name="reason" maxLength={200} required /></Field><button className="button primary" disabled={mutation.isPending}>{cancelling ? '确认取消此次绑定' : '确认撤销此绑定'}</button></form></Dialog>;
}

function Channels({ bots }: { bots: Bot[] }) {
  const query = useInfiniteQuery({ queryKey: ['telegram', 'channels'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Channel>>('/telegram/channels?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const [editing, setEditing] = useState<Channel | 'new'>(), [history, setHistory] = useState<Channel>();
  const rows = query.data?.pages.flatMap(page => page.items) ?? [];
  return <section className="panel"><div className="section-heading"><h2>群与话题白名单</h2><button className="button secondary" disabled={!bots.length} onClick={() => setEditing('new')}>新增群入口</button></div><p className="form-note">每个入口对应一个准确群号和话题。新入口先暂停，核对后才能接收明确请求；成员或群标识变化会暂停接收。严格受限业务内容需登录门户查阅。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取群入口" /> : rows.map(channel => <article className="telegram-entry" key={channel.id}><h3>{channel.label}</h3><p>{channel.state === 'active' ? '接收已启用' : '接收已暂停'} · 第 {channel.version} 版</p><p>群号：<code>{channel.chat_id}</code> · 话题：{channel.topic_id === 0 ? '无话题' : channel.topic_id}</p><p>允许任务：{channel.allowed_skill_ids.map(id => skillNames[id]).join('、')}</p><button className="button secondary" onClick={() => setEditing(channel)}>编辑群入口</button><button className="button secondary" onClick={() => setHistory(channel)}>查看入口历史</button></article>)}{!query.isPending && !rows.length && !query.error && <p>尚未配置群与话题白名单。</p>}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多群入口</button>}
    {editing && <ChannelEditor value={editing === 'new' ? undefined : editing} bots={bots} onClose={() => setEditing(undefined)} />}{history && <ChannelHistory value={history} onClose={() => setHistory(undefined)} />}
  </section>;
}
function ChannelEditor({ value, bots, onClose }: { value?: Channel; bots: Bot[]; onClose(): void }) {
  const [projectSearch, setProjectSearch] = useState(''), [projectID, setProjectID] = useState(value?.project_id ?? '');
  const projects = useInfiniteQuery({ queryKey: ['telegram', 'project-options', projectSearch], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['Project']>>('/projects?' + new URLSearchParams({ limit: '20', ...(projectSearch ? { q: projectSearch } : {}), ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const projectItems = projects.data?.pages.flatMap(page => page.items) ?? [];
  const [state, setState] = useState(value?.state ?? 'paused');
  const [allowed, setAllowed] = useState<Channel['allowed_skill_ids']>(value?.allowed_skill_ids ?? ['project-briefing']), [defaultSkill, setDefaultSkill] = useState(value?.default_skill_id ?? '');
  const mutation = useWrite<Channel>(value ? '/telegram/channels/' + value.id : '/telegram/channels', onClose);
  return <Dialog title={value ? '编辑指定群入口' : '新增指定群入口'} onClose={onClose} busy={mutation.isPending}><form className="telegram-form" onSubmit={event => { event.preventDefault(); const form = new FormData(event.currentTarget), project = String(form.get('project') ?? ''); const common = { label: String(form.get('label')), project_id: project || null, allowed_skill_ids: form.getAll('skill').map(String), default_skill_id: defaultSkill || null, delivery_enabled: form.get('delivery') === 'on', reason: String(form.get('reason')) }; mutation.mutate(value ? { ...common, expected_version: value.version, state } : { ...common, bot_id: String(form.get('bot')), chat_id: String(form.get('chat')), topic_id: Number(form.get('topic')) }); }}><ErrorNotice error={mutation.error} /><ErrorNotice error={projects.error} />
    {value ? <p>准确群号 {value.chat_id} · 话题 {value.topic_id}。更换位置须另建入口。</p> : <><Field label="接收Bot"><select name="bot" defaultValue="" required><option value="" disabled>请明确选择Bot</option>{bots.map(bot => <option key={bot.bot_id} value={bot.bot_id}>@{bot.username}</option>)}</select></Field><Field label="数字群号"><input name="chat" pattern="-[1-9][0-9]{0,15}" required /></Field><Field label="数字话题号" hint="无话题填0；其他话题不会被这个入口接收。"><input name="topic" type="number" min={0} max={2147483647} defaultValue={0} required /></Field></>}
    <Field label="入口名称"><input name="label" defaultValue={value?.label ?? ''} maxLength={200} required /></Field><Field label="查找项目"><input type="search" value={projectSearch} onChange={event => setProjectSearch(event.target.value)} maxLength={200} /></Field><Field label="项目范围"><select name="project" value={projectID} onChange={event => setProjectID(event.target.value)}><option value="">公司范围</option>{projectID && !projectItems.some(project => project.id === projectID) && <option value={projectID}>已选择项目（请核对当前权限）</option>}{projectItems.map(project => <option key={project.id} value={project.id}>{project.name}</option>)}</select></Field>{projects.hasNextPage && <button type="button" className="button secondary" disabled={projects.isFetchingNextPage} onClick={() => void projects.fetchNextPage()}>加载更多可选项目</button>}
    <Field label="允许任务">{Object.entries(skillNames).map(([id, label]) => <label key={id}><input name="skill" type="checkbox" value={id} checked={allowed.includes(id as keyof typeof skillNames)} onChange={event => { setAllowed(items => event.target.checked ? [...items, id as keyof typeof skillNames] : items.filter(item => item !== id)); if (!event.target.checked && defaultSkill === id) setDefaultSkill(''); }} />{label}</label>)}</Field>
    <Field label="默认任务" hint="/brief指定项目简报，/incident指定事件摘要；/ask、提及和回复使用默认任务。"><select value={defaultSkill} onChange={event => setDefaultSkill(event.target.value)}><option value="">按唯一允许任务；多项须明确命令</option>{allowed.map(id => <option key={id} value={id}>{skillNames[id]}</option>)}</select></Field>
    {value && <Field label="接收状态"><select value={state} onChange={event => setState(event.target.value as Channel['state'])}><option value="paused">暂停接收</option><option value="active">启用接收</option></select></Field>}{value && state === 'active' && <Field label="范围确认"><label><input type="checkbox" required />我已核对群与话题，并允许接收指定任务</label></Field>}
    <Field label="群通知"><label><input name="delivery" type="checkbox" defaultChecked={value?.delivery_enabled ?? false} />允许向此群与话题发送受理、进度和结果登录链接</label><p className="form-note">通知正文不含请求与业务内容。停用或修改群入口会阻止尚未发出的通知。</p></Field><Field label="配置原因"><input name="reason" maxLength={200} required /></Field><p className="form-note">此操作需要近期两步验证。群内不提供批准业务写入的按钮；接收请求不代表已启动秘书或已完成业务。</p><button className="button primary" disabled={mutation.isPending}>{value ? '保存此群入口配置' : '创建暂停的群入口'}</button>
  </form></Dialog>;
}
function ChannelHistory({ value, onClose }: { value: Channel; onClose(): void }) {
  const query = useInfiniteQuery({ queryKey: ['telegram', 'history', value.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['TelegramChannelRevision']>>('/telegram/channels/' + value.id + '/history?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  return <Dialog title="群入口变更历史" onClose={onClose}><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取入口历史" /> : query.data?.pages.flatMap(page => page.items).map(revision => <article className="telegram-entry" key={revision.id}><h3>第 {revision.version} 版 · {revision.snapshot.state === 'active' ? '接收启用' : '接收暂停'}</h3><p>{revision.source === 'portal' ? '门户配置' : revision.source === 'migration' ? '群标识变化' : '群成员变化'} · {date(revision.created_at)}</p><p>{revision.reason}</p></article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早入口历史</button>}</Dialog>;
}
