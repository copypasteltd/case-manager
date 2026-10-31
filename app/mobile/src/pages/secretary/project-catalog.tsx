import { Text, View } from '@tarojs/components';
import { useEffect, useState } from 'react';
import { catalogSourceMatches, type CatalogItem, type ProjectSelection } from '@copypaste/contracts/project-catalog';
import { Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice } from '../../ui';
import { TextField } from '../business/controls';

export function ProjectCatalogSelection({ conversation: c, me, value, onChange, disabled }: { conversation: Schemas['Conversation']; me: Schemas['Me']; value: ProjectSelection; onChange(v: ProjectSelection): void; disabled: boolean }) {
  return <View><TextField name='plan-type' label='项目类型' value={value.type} onChange={type => onChange({ ...value, type })} disabled={disabled} max={100} />
    <TextField name='plan-start' label='开始日期（YYYY-MM-DD，可选）' value={value.start} onChange={start => onChange({ ...value, start })} disabled={disabled} max={10} />
    <TextField name='plan-due' label='截止日期（YYYY-MM-DD，可选）' value={value.due} onChange={due => onChange({ ...value, due })} disabled={disabled} max={10} />
    <Fact label='项目负责人'>{value.owner?.name ?? me.user.display_name}{!value.owner && '（本人）'}</Fact>
    {value.owner && <Action secondary disabled={disabled} onClick={() => onChange({ ...value, owner: undefined })}>恢复本人负责</Action>}
    <Fact label='额外成员'>{value.members.length ? value.members.map(v => v.name).join('、') : '未添加'}</Fact>
    {value.members.map(v => <Action key={v.id} secondary disabled={disabled} onClick={() => onChange({ ...value, members: value.members.filter(m => m.id !== v.id) })}>移除成员 {v.name}</Action>)}
    {me.actions.includes('project.manage_members') && <CatalogPicker key={'people:' + c.id + ':' + me.authorization_version} conversation={c} kind='people' disabled={disabled} onPick={(v, owner) => onChange(owner ? { ...value, owner: v, members: value.members.filter(m => m.id !== v.id) } : { ...value, members: [...value.members.filter(m => m.id !== v.id), v] })} />}
    <Fact label='项目模板'>{value.template?.name ?? '未指定，使用标准目录'}</Fact>
    {value.template && <><Action secondary disabled={disabled} onClick={() => onChange({ ...value, template: undefined })}>移除模板</Action><CatalogSource conversation={c} source={{ type: value.template.type, id: value.template.id, resource_version: value.template.version, sha256: value.template.source_sha256 }} /></>}
    {me.actions.includes('project.create') && <CatalogPicker key={'templates:' + c.id + ':' + me.authorization_version} conversation={c} kind='templates' disabled={disabled} onPick={template => onChange({ ...value, template })} />}
    <Text className='cp-paragraph'>目录仅用于选择候选。保存后须核对固定计划，再由本人批准和创建；模板中的任务与目录在创建时一并生成。</Text>
  </View>;
}
function CatalogPicker({ conversation: c, kind, onPick, disabled }: { conversation: Schemas['Conversation']; kind: 'people' | 'templates'; onPick(v: CatalogItem, owner?: boolean): void; disabled: boolean }) {
  const [open, setOpen] = useState(false), [input, setInput] = useState(''), [query, setQuery] = useState(''), [cursor, setCursor] = useState(''), [revision, setRevision] = useState(0), [value, setValue] = useState<Schemas['AgentProjectCatalogPage']>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(false);
  useEffect(() => { const scope = new Scope(); setValue(undefined); setError(undefined); setLoading(open && !disabled);
    if (open && !disabled) void client.request<Schemas['AgentProjectCatalogPage']>('/conversations/' + c.id + '/project-catalog?kind=' + kind + '&query=' + encodeURIComponent(query) + '&cursor=' + encodeURIComponent(cursor) + '&limit=20', { scope }).then(v => { if (!scope.closed) setValue(v); }).catch(e => { if (!scope.closed) setError(e); }).finally(() => { if (!scope.closed) setLoading(false); });
    return () => scope.close();
  }, [c.id, c.current_revision_id, kind, query, cursor, open, disabled, revision]);
  const label = kind === 'people' ? '人员' : '模板';
  return <View className='cp-inset'><Action secondary disabled={disabled} onClick={() => setOpen(v => !v)}>{open ? '收起' : '选择'}{label}目录</Action>
    {open && <><TextField name={'catalog-' + kind} label={label + '关键词或准确编号'} value={input} onChange={setInput} disabled={disabled} /><Action secondary disabled={disabled || loading} onClick={() => { setQuery(input.trim()); setCursor(''); setRevision(v => v + 1); }}>查询{label}</Action>
      <Notice error={error} />{loading ? <Busy /> : !error && value && <>{value.items.map(v => <View key={v.id} className='cp-inset'><Text className='cp-section-title'>{v.name}{v.revision != null && ' · 修订 ' + v.revision}</Text><Text className='cp-paragraph'>{v.description}</Text><Text selectable className='cp-code'>{v.id}</Text>
        {kind === 'templates' ? <><Text>{v.folder_count}个模板目录 · {v.task_count}项任务 · {v.milestone_count}个里程碑</Text><Action secondary disabled={disabled} onClick={() => onPick(v)}>使用此模板</Action></> : <><Action secondary disabled={disabled} onClick={() => onPick(v, true)}>设为负责人</Action><Action secondary disabled={disabled} onClick={() => onPick(v)}>加入项目成员</Action></>}</View>)}
        {!value.items.length && <Empty>没有匹配的{label}，请核对关键词、权限或发布状态。</Empty>}{value.next_cursor && <Action secondary disabled={disabled} onClick={() => setCursor(value.next_cursor!)}>下一页{label}</Action>}{cursor && <Action secondary disabled={disabled} onClick={() => setCursor('')}>返回首批{label}</Action>}</>}
    </>}
  </View>;
}
export function CatalogSource({ conversation: c, source }: { conversation: Schemas['Conversation']; source: Pick<Schemas['AgentSourceSnapshot'], 'type' | 'id' | 'resource_version' | 'sha256'> }) {
  const [open, setOpen] = useState(false), [offset, setOffset] = useState(0), [value, setValue] = useState<Schemas['AgentProjectCatalogDocument']>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(false);
  useEffect(() => { const scope = new Scope(); setValue(undefined); setError(undefined); setLoading(open);
    if (open) void client.request<Schemas['AgentProjectCatalogDocument']>('/conversations/' + c.id + '/project-catalog/' + source.type + '/' + source.id + '?offset=' + offset + '&max_chars=4000', { scope }).then(v => { if (!scope.closed) { if (!catalogSourceMatches(v, source)) throw new Error('人员或模板已变化，原目录引用已隐藏，请重新查询。'); setValue(v); } }).catch(e => { if (!scope.closed) setError(e); }).finally(() => { if (!scope.closed) setLoading(false); });
    return () => scope.close();
  }, [c.id, c.current_revision_id, source.id, source.type, source.sha256, source.resource_version, offset, open]);
  return <View><Action secondary onClick={() => setOpen(v => !v)}>{open ? '收起目录内容' : '核对准确目录内容'}</Action>{open && <><Notice error={error} />{loading ? <Busy /> : !error && value && <><Fact label='准确目录资料'>{value.item.name} · {value.offset + 1}–{value.offset + [...value.text].length}／{value.total_chars}字符</Fact><Text selectable className='cp-code'>{value.text}</Text>{value.next_offset != null && <Action secondary onClick={() => setOffset(value.next_offset!)}>继续读取目录内容</Action>}{offset > 0 && <Action secondary onClick={() => setOffset(0)}>回到正文开头</Action>}</>}</>}</View>;
}
