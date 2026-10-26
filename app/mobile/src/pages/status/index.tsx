import { Text, View } from '@tarojs/components';
import Taro, { useDidHide, useRouter } from '@tarojs/taro';
import { useState } from 'react';
import { useCollection, useRecord } from '../../platform/collection';
import { go, identifier, usePage } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { incidentNames, severityNames, sourceReasons, statusIsCurrent, statusNames, statusTime, statusTones, type StatusSnapshot } from '../../platform/status';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';
import { IncidentEditor, type IncidentSeed } from './incident-editor';
import { Maintenance, Observations, Revisions } from './history';
import { useLiveStatus } from './live';

export default function StatusPage() {
  const params = useRouter().params, [editing, setEditing] = useState(false), [rebased, setRebased] = useState<{ value: Schemas['Incident']; seed: IncidentSeed }>();
  useDidHide(() => { setEditing(false); setRebased(undefined); });
  const page = usePage(async scope => {
    const kind = params.kind ?? 'services';
    if (!['services', 'service', 'incidents', 'incident', 'new-incident'].includes(kind)) throw new MobileError('INVALID_RESOURCE', '状态资料地址无效，请从工作台重新打开。');
    const id = kind === 'service' || kind === 'incident' ? identifier(params.id) : undefined;
    const incident = kind === 'incident' ? await client.request<Schemas['Incident']>('/incidents/' + id, { scope }) : undefined;
    if (incident && incident.id !== id) throw new MobileError('INVALID_RESPONSE', '事件与所选地址不一致，请重新读取。');
    return { kind, id, incident };
  });
  const result = page.result, data = result?.value;
  const editable = rebased && rebased.value.id === data?.incident?.id ? rebased.value : data?.incident;
  const title = data?.kind === 'service' ? '服务详情' : data?.kind === 'incident' ? data.incident?.title ?? '事件详情' : data?.kind === 'new-incident' ? '登记服务事件' : data?.kind === 'incidents' ? '服务事件' : '服务状态';
  return <Page title={title} eyebrow='OPERATIONS / THE CURRENT FACTS' home>
    <Notice error={page.error} />{page.loading ? <Busy /> : result && data && <>
      <View className='cp-status-navigation'><Action secondary onClick={() => void Taro.redirectTo({ url: '/pages/status/index?kind=services' })}>服务目录</Action><Action secondary onClick={() => void Taro.redirectTo({ url: '/pages/status/index?kind=incidents' })}>事件记录</Action></View>
      {data.kind === 'services' && <Services />}
      {data.kind === 'service' && <ServiceDetail id={data.id!} me={result.me} />}
      {data.kind === 'incidents' && <Incidents canCreate={result.me.actions.includes('incident.update')} />}
      {data.kind === 'new-incident' && (result.me.actions.includes('incident.update') ? <IncidentEditor me={result.me} onCommitted={value => void Taro.redirectTo({ url: '/pages/status/index?kind=incident&id=' + identifier(value.id) })} onCancel={() => void Taro.redirectTo({ url: '/pages/status/index?kind=incidents' })} /> : <Empty>当前权限不允许登记事件。</Empty>)}
      {data.kind === 'incident' && data.incident && (editing && editable?.can_update ? <IncidentEditor key={editable.id + ':' + editable.version + ':' + result.me.authorization_version} me={result.me} current={editable} seed={rebased?.seed} onRebase={(value, seed) => setRebased({ value, seed })} onReload={() => { setRebased(undefined); setEditing(false); void page.reload(); }} onCommitted={() => { setRebased(undefined); setEditing(false); void page.reload(); }} onCancel={() => { setRebased(undefined); setEditing(false); void page.reload(); }} /> : <IncidentDetail incident={data.incident} me={result.me} onEdit={() => setEditing(true)} />)}
    </>}
    <Action secondary loading={page.loading} onClick={() => { setRebased(undefined); setEditing(false); void page.reload(); }}>重新读取当前权限与资料</Action>
  </Page>;
}
function Badge({ snapshot, now }: { snapshot: StatusSnapshot; now: number }) {
  const current = statusIsCurrent(snapshot, now);
  return <Text className={'cp-state cp-state-' + (current ? statusTones[snapshot.value.state] : 'neutral')}>{current ? statusNames[snapshot.value.state] : '待重新确认'}</Text>;
}
function Services() {
  const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [lifecycle, setLifecycle] = useState('active');
  const [project, setProject] = useState<Schemas['Project']>(), [picking, setPicking] = useState(false);
  const address = '/services?limit=20' + (query ? '&q=' + encodeURIComponent(query) : '') + (lifecycle ? '&lifecycle=' + lifecycle : '') + (project ? '&project_id=' + project.id : '');
  const feed = useLiveStatus(address);
  if (picking) return <ChoiceList<Schemas['Project']> title='选择服务所属项目' path='/projects?limit=20' label={item => item.name} selected={project?.id} onSelect={value => { setProject(value); setPicking(false); }} onClose={() => setPicking(false)} />;
  return <>
    <Section title='查找服务'><View className='cp-pad'><TextField name='service-search' label='服务名称或展示信息' value={search} onChange={setSearch} disabled={false} /><Action secondary onClick={() => { setQuery(search.trim()); feed.reload(); }}>查找服务</Action>
      <SelectField label='服务登记阶段' value={lifecycle} options={[[ 'active', '使用中' ], [ 'archived', '已归档' ], [ '', '全部可见阶段' ]]} onChange={setLifecycle} disabled={false} />
      <Text className='cp-muted cp-small cp-paragraph'>所属项目：{project?.name ?? '全部有权查看的项目与公司服务'}</Text>
      <Action secondary onClick={() => setPicking(true)}>按项目筛选服务</Action>{project && <Action secondary onClick={() => setProject(undefined)}>清除项目筛选</Action>}
    </View></Section>
    <Notice error={feed.error} /><ClockNotice available={feed.clockAvailable} />
    <Section title='当前可见服务'>{feed.loading ? <Busy /> : feed.items.map(snapshot => <View key={snapshot.value.id} className='cp-editor-row cp-pad'>
      <Row title={snapshot.value.name} subtitle={snapshot.value.lifecycle === 'archived' ? '已归档服务' : snapshot.value.description || '查看观测、来源与维护记录'} onClick={() => go('service', snapshot.value.id)} />
      <View className='cp-choice'><Badge snapshot={snapshot} now={feed.now} /><Text className='cp-muted cp-small'>评估于 {statusTime(snapshot.value.evaluated_at)}</Text></View>
    </View>)}{!feed.loading && !feed.error && !feed.items.length && <Empty>没有匹配的可见服务。服务登记与监控来源由管理端维护。</Empty>}</Section>
    {feed.next && <Action secondary loading={feed.busy} onClick={() => void feed.more()}>加载更多服务</Action>}
    <Action secondary onClick={feed.reload}>刷新服务目录与分页</Action>
  </>;
}
function ClockNotice({ available }: { available: boolean }) {
  return <Text className='cp-notice cp-paragraph'>{available ? '自动重新读取当前已载入的服务。到达重读时刻后，旧状态会显示为待重新确认。' : '当前设备无法确认观测时效。以下仅为服务器最后返回的记录，不显示为当前正常状态。'}</Text>;
}
function ServiceDetail({ id, me }: { id: string; me: Schemas['Me'] }) {
  const feed = useLiveStatus('/services/' + id, true), snapshot = feed.items[0], value = snapshot?.value;
  const [view, setView] = useState('overview');
  const current = snapshot && statusIsCurrent(snapshot, feed.now), source = value?.sources.find(item => view === item.id);
  return <><Notice error={feed.error} /><ClockNotice available={feed.clockAvailable} />
    {feed.loading ? <Busy /> : value && snapshot && <>
      <Section title={value.name}><View className='cp-pad'><Badge snapshot={snapshot} now={feed.now} />
        {!current && <Text className='cp-notice cp-paragraph'>上次评估已到重读时刻，当前状态待重新确认。下方保留的是上次取得的证据。</Text>}
        <Fact label='最后评估'><Text>{statusTime(value.evaluated_at)}</Text></Fact><Fact label='最迟重新读取'><Text>{statusTime(value.recheck_at)}</Text></Fact>
        <Fact label={current ? '底层观测状态' : '上次底层观测'}><Text>{statusNames[value.observed_state]}</Text></Fact>
        <Fact label='登记阶段'><Text>{value.lifecycle === 'archived' ? '已归档' : '使用中'}</Text></Fact>
        <Fact label='服务说明'><Text>{value.description || '未填写'}</Text></Fact><Fact label='负责人'><Text>{value.owner_id === me.user.id ? me.user.display_name : '组织成员'}</Text><Text className='cp-code'>{value.owner_id}</Text></Fact>
        <Fact label='主机展示名称'><Text>{value.host_label || '未填写'}</Text></Fact><Fact label='端点展示名称'><Text>{value.endpoint_label || '未填写'}</Text></Fact>
        <Fact label='未取得有效观测的来源'><Text>{value.unknown_sources} 项</Text></Fact><Fact label='配置修订'><Text>{value.version}</Text></Fact>
        {value.maintenance_id && <Text className='cp-notice cp-paragraph'>{current ? '当前评估包含维护覆盖，底层观测仍单独保留。' : '上次评估包含维护覆盖；目前是否仍在维护需重新确认。'}</Text>}
        {value.project_id && <Action secondary onClick={() => go('project', value.project_id!)}>查看所属项目</Action>}
      </View></Section>
      <View className='cp-status-navigation'><Action secondary onClick={() => setView('overview')}>来源概览</Action><Action secondary onClick={() => setView('observations')}>查看观测事实</Action><Action secondary onClick={() => setView('maintenance')}>查看维护记录</Action></View>
      {view === 'observations' ? <Observations serviceID={id} /> : view === 'maintenance' ? <Maintenance serviceID={id} /> : source ? <Revisions serviceID={id} source={source} /> : <Section title='可信观测来源'><View className='cp-pad'>
        {!value.sources.length && <Empty>尚无监控数据。登记服务本身不会产生正常状态。</Empty>}
        {value.sources.map(item => <View className='cp-editor-row' key={item.id}>
          <Text className='cp-row-title'>{item.name}</Text><Text className='cp-paragraph'>{item.enabled ? '来源启用' : '来源停用'} · {({ uptime_kuma: 'Uptime Kuma', prometheus: 'Prometheus', adapter: '已登记适配器' }[item.provider])}</Text>
          <Fact label={current ? '来源评估' : '上次来源评估'}><Text>{current ? statusNames[item.state] : '待重新确认'}</Text></Fact>
          {item.state_reason && <Text className='cp-muted cp-small cp-paragraph'>{sourceReasons[item.state_reason] ?? '来源状态待核对'}</Text>}
          <Fact label='最后观测'><Text>{statusTime(item.projection?.observed_at)}</Text></Fact><Fact label='最后接收'><Text>{statusTime(item.projection?.received_at)}</Text></Fact>
          <Fact label='观测序号'><Text>{item.projection?.sequence ?? '尚无有效观测'}</Text></Fact><Fact label='探测／过期阈值'><Text>{item.interval_seconds}／{item.stale_after_seconds} 秒</Text></Fact>
          <Fact label='连续失败／恢复'><Text>{item.failure_count}／{item.recovery_count} 次</Text></Fact>
          <Action secondary onClick={() => setView(item.id)}>查看来源修订历史</Action>
        </View>)}
      </View></Section>}
    </>}
    <Action secondary onClick={feed.reload}>刷新状态与依据</Action>
  </>;
}
function Incidents({ canCreate }: { canCreate: boolean }) {
  const [state, setState] = useState(''), query = useCollection<Schemas['Incident']>('/incidents?limit=20' + (state ? '&state=' + state : ''));
  return <>{canCreate && <Action onClick={() => go('new-incident')}>登记服务事件</Action>}
    <SelectField label='事件处理阶段筛选' value={state} options={[[ '', '全部可见事件' ], ...Object.entries(incidentNames)]} onChange={setState} disabled={false} /><Notice error={query.error} />
    <Section title='事件记录'>{query.loading ? <Busy /> : query.items.map(item => <Row key={item.id} title={item.title} subtitle={incidentNames[item.state] + ' · 严重程度：' + severityNames[item.severity] + ' · ' + statusTime(item.updated_at)} onClick={() => go('incident', item.id)} />)}
      {!query.loading && !query.error && !query.items.length && <Empty>没有符合条件且全部影响范围可见的事件。</Empty>}
    </Section>{query.next && <Action secondary loading={query.moreBusy} onClick={() => void query.more()}>加载更多事件</Action>}
    <Action secondary onClick={query.reload}>刷新事件记录</Action>
  </>;
}
function IncidentDetail({ incident, me, onEdit }: { incident: Schemas['Incident']; me: Schemas['Me']; onEdit(): void }) {
  const timeline = useCollection<Schemas['IncidentEntry']>('/incidents/' + incident.id + '/timeline?limit=20');
  return <><Section title='事件当前记录'><View className='cp-pad'>
    <Text className='cp-state cp-state-info'>{incidentNames[incident.state]}</Text><Fact label='严重程度'><Text>{severityNames[incident.severity]}</Text></Fact>
    <Fact label='事件影响描述'><Text>{incident.description || '未填写'}</Text></Fact><Fact label='负责人'><Text>{incident.owner_id === me.user.id ? me.user.display_name : '组织成员'}</Text><Text className='cp-code'>{incident.owner_id}</Text></Fact>
    <Fact label='当前修订'><Text>{incident.version}</Text></Fact><Fact label='最近更新'><Text>{statusTime(incident.updated_at)}</Text></Fact><Fact label='首次解决时间'><Text>{statusTime(incident.resolved_at)}</Text></Fact>
    <Fact label='恢复与解决说明'><Text className='cp-status-text'>{incident.resolution || '尚未登记'}</Text></Fact><Fact label='事件复盘'><Text className='cp-status-text'>{incident.postmortem || '尚未登记'}</Text></Fact>
    {incident.can_update && <Action onClick={onEdit}>追加事件进展</Action>}
  </View></Section><Section title='受影响服务'>{incident.service_ids.map(id => <ServiceReference key={id} id={id} />)}</Section>
    <Section title='不可变事件时间线'><View className='cp-pad'><Notice error={timeline.error} />{timeline.loading ? <Busy /> : timeline.items.map(item => <View className='cp-editor-row' key={item.id}>
      <Text className='cp-row-title'>记录 {item.seq} · {incidentNames[item.state]}</Text><Text className='cp-muted cp-small cp-paragraph'>{statusTime(item.created_at)}</Text><Text className='cp-status-text cp-paragraph'>{item.note}</Text>
      <Fact label='当时解决说明'><Text className='cp-status-text'>{item.resolution || '当时未登记'}</Text></Fact><Fact label='当时复盘'><Text className='cp-status-text'>{item.postmortem || '当时未登记'}</Text></Fact><Fact label='记录人'><Text className='cp-code'>{item.actor_id}</Text></Fact>
    </View>)}{!timeline.loading && !timeline.error && !timeline.items.length && <Empty>尚无可见时间线记录。</Empty>}
      {timeline.next && <Action secondary loading={timeline.moreBusy} onClick={() => void timeline.more()}>加载更早事件进展</Action>}
    </View></Section></>;
}
function ServiceReference({ id }: { id: string }) {
  const query = useRecord<Schemas['Service']>('/services/' + id);
  return query.loading ? <Busy /> : query.value ? <Row title={query.value.name} subtitle='查看当前服务观测与依据' onClick={() => go('service', id)} /> : <Notice error={query.error} />;
}
