import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, type Me, type Schemas } from './api';
import { Busy, Empty, ErrorNotice, Field } from './components';
import { catalogSourceMatches, type CatalogItem, type ProjectSelection } from '@copypaste/contracts/project-catalog';

export function ProjectCatalogSelection({ conversation: c, me, value, onChange, disabled }: { conversation: Schemas['Conversation']; me: Me; value: ProjectSelection; onChange(v: ProjectSelection): void; disabled: boolean }) {
  return <><Field label="项目类型"><input maxLength={100} value={value.type} disabled={disabled} onChange={e => onChange({ ...value, type: e.target.value })} /></Field>
    <Field label="开始日期"><input type="date" value={value.start} disabled={disabled} onChange={e => onChange({ ...value, start: e.target.value })} /></Field>
    <Field label="截止日期"><input type="date" value={value.due} disabled={disabled} onChange={e => onChange({ ...value, due: e.target.value })} /></Field>
    <p>项目负责人：{value.owner?.name ?? me.user.display_name}{!value.owner && '（本人）'} {value.owner && <button type="button" className="button secondary" disabled={disabled} onClick={() => onChange({ ...value, owner: undefined })}>恢复本人负责</button>}</p>
    <p>额外成员：{value.members.length ? value.members.map(v => <span key={v.id}>{v.name} <button type="button" className="button secondary" disabled={disabled} onClick={() => onChange({ ...value, members: value.members.filter(m => m.id !== v.id) })}>移除成员</button> </span>) : '未添加'}</p>
    {me.actions.includes('project.manage_members') && <CatalogPicker key={'people:' + c.id + ':' + me.authorization_version} conversation={c} me={me} kind="people" disabled={disabled} onPick={(v, owner) => onChange(owner ? { ...value, owner: v, members: value.members.filter(m => m.id !== v.id) } : { ...value, members: [...value.members.filter(m => m.id !== v.id), v] })} />}
    <p>项目模板：{value.template?.name ?? '未指定，使用标准目录'} {value.template && <button type="button" className="button secondary" disabled={disabled} onClick={() => onChange({ ...value, template: undefined })}>移除模板</button>}</p>
    {value.template && <CatalogSource conversation={c} source={{ type: value.template.type, id: value.template.id, resource_version: value.template.version, sha256: value.template.source_sha256 }} />}
    {me.actions.includes('project.create') && <CatalogPicker key={'templates:' + c.id + ':' + me.authorization_version} conversation={c} me={me} kind="templates" disabled={disabled} onPick={v => onChange({ ...value, template: v })} />}
    <p className="form-note">目录仅用于选择候选。保存后仍须核对固定计划，再由本人批准和创建；模板中的任务与目录将在创建时一并生成。</p>
  </>;
}
function CatalogPicker({ conversation: c, me, kind, onPick, disabled }: { conversation: Schemas['Conversation']; me: Me; kind: 'people' | 'templates'; onPick(item: CatalogItem, owner?: boolean): void; disabled: boolean }) {
  const [input, setInput] = useState(''), [query, setQuery] = useState(''), [cursor, setCursor] = useState(''), [open, setOpen] = useState(false);
  const page = useQuery({ queryKey: ['project-catalog', c.id, c.context_security_version, me.authorization_version, kind, query, cursor], enabled: open && !disabled, gcTime: 0, retry: false,
    queryFn: ({ signal }) => api<Schemas['AgentProjectCatalogPage']>('/conversations/' + c.id + '/project-catalog?' + new URLSearchParams({ kind, query, cursor, limit: '20' }), { signal }) });
  const label = kind === 'people' ? '人员' : '模板';
  return <section className="conversation-source-preview"><button type="button" className="button secondary" disabled={disabled} onClick={() => setOpen(v => !v)}>{open ? '收起' : '选择'}{label}目录</button>
    {open && <><Field label={label + '关键词或准确编号'}><input maxLength={200} value={input} disabled={disabled} onChange={e => setInput(e.target.value)} /></Field><button type="button" className="button secondary" disabled={disabled || page.isFetching} onClick={() => { if (query === input.trim() && !cursor) void page.refetch(); else { setQuery(input.trim()); setCursor(''); } }}>查询{label}</button>
      <ErrorNotice error={page.error} />{page.isFetching ? <Busy /> : !page.error && page.data && <>{page.data.items.map(item => <article key={item.id}><p><strong>{item.name}</strong>{item.revision != null && ' · 修订 ' + item.revision}</p><p>{item.description}</p><code>{item.id}</code>
        {kind === 'templates' ? <><p>{item.folder_count}个模板目录 · {item.task_count}项任务 · {item.milestone_count}个里程碑</p><button type="button" className="button secondary" disabled={disabled} onClick={() => onPick(item)}>使用此模板</button></> : <div className="actions"><button type="button" className="button secondary" disabled={disabled} onClick={() => onPick(item, true)}>设为负责人</button><button type="button" className="button secondary" disabled={disabled} onClick={() => onPick(item)}>加入项目成员</button></div>}</article>)}
        {!page.data.items.length && <Empty title={'没有匹配的' + label} detail="请核对关键词、当前权限或发布状态。" />}{page.data.next_cursor && <button type="button" className="button secondary" disabled={disabled} onClick={() => setCursor(page.data!.next_cursor!)}>下一页{label}</button>}{cursor && <button type="button" className="button secondary" onClick={() => setCursor('')}>返回首批{label}</button>}</>}
    </>}
  </section>;
}
export function CatalogSource({ conversation: c, source }: { conversation: Schemas['Conversation']; source: Pick<Schemas['AgentSourceSnapshot'], 'type' | 'id' | 'resource_version' | 'sha256'> }) {
  const [open, setOpen] = useState(false), [offset, setOffset] = useState(0);
  const current = useQuery({ queryKey: ['project-catalog-source', c.id, c.current_revision_id, source.id, source.sha256, offset], enabled: open, gcTime: 0, retry: false,
    queryFn: async ({ signal }) => { const v = await api<Schemas['AgentProjectCatalogDocument']>('/conversations/' + c.id + '/project-catalog/' + source.type + '/' + source.id + '?offset=' + offset + '&max_chars=4000', { signal }); if (!catalogSourceMatches(v, source)) throw new Error('人员或模板已变化，原目录引用已隐藏，请重新查询。'); return v; } });
  return <div><button type="button" className="button secondary" onClick={() => setOpen(v => !v)}>{open ? '收起目录内容' : '核对准确目录内容'}</button>{open && <><ErrorNotice error={current.error} />{current.isFetching ? <Busy /> : !current.error && current.data && <><p>{current.data.item.name} · {current.data.offset + 1}–{current.data.offset + [...current.data.text].length}／{current.data.total_chars}字符</p><pre>{current.data.text}</pre>{current.data.next_offset != null && <button type="button" className="button secondary" onClick={() => setOffset(current.data!.next_offset!)}>继续读取目录内容</button>}{offset > 0 && <button type="button" className="button secondary" onClick={() => setOffset(0)}>回到正文开头</button>}</>}</>}</div>;
}
