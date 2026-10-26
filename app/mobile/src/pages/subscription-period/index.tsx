import { Text } from '@tarojs/components';
import { useRouter } from '@tarojs/taro';
import { client } from '../../platform/taro';
import { go, identifier, usePage } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Fact, Notice, Page, Section, State, dateOnly } from '../../ui';
export default function SubscriptionPeriodPage() {
  const params = useRouter().params;
  const page = usePage(async scope => {
    const id = identifier(params.id), value = await client.request<Schemas['SubscriptionPeriod']>('/subscription-periods/' + id, { scope });
    if (value.id !== id) throw new MobileError('INVALID_RESPONSE', '账期与所选资料不一致。');
    return value;
  });
  const value = page.result?.value;
  return <Page title={value?.product ?? '订阅账期'} home><Notice error={page.error} />{page.loading ? <Busy /> : value && <Section title='当前账期'><Fact label='账期状态'><State value={value.state} /></Fact><Fact label='计费日期'><Text>{dateOnly(value.start_date)}</Text></Fact><Fact label='账期结束'><Text>{dateOnly(value.end_date)}</Text></Fact><Fact label='业务时区'><Text>{value.timezone}</Text></Fact><Fact label='预计金额'><Text>{value.currency} {value.amount}</Text></Fact><Fact label='已核对实际金额'><Text>{value.actual_amount === null ? '尚未登记' : value.currency + ' ' + value.actual_amount}</Text></Fact><Fact label='核对与费用登记'><Text>请在 Web 工作空间核对账期、关联实际费用并处理差异。通知已读不会登记付款。</Text></Fact></Section>}{value && <><Action secondary onClick={() => go('subscription', value.subscription_id)}>查看订阅配置与价格期间</Action>{value.cost_ids.map(id => <Action key={id} secondary onClick={() => go('cost', id)}>查看对应实际费用</Action>)}</>}<Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新账期</Action></Page>;
}
