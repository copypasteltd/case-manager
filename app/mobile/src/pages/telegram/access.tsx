import { Text, View } from '@tarojs/components';
import Taro, { useDidHide } from '@tarojs/taro';
import { useState } from 'react';
import { client } from '../../platform/taro';
import { copyTelegramProof } from '../../platform/telegram-clipboard';
import { usePage } from '../../platform/page';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { bindingReceipt, challengeReceipt, endedReceipt, telegramReason, type Binding, type Challenge } from '../../platform/telegram-input';
import type { Schemas } from '../../platform/session';
import { TextField, SelectField } from '../business/controls';
import { TelegramChannels } from './channels';
import { Action, Busy, Check, Empty, Fact, Notice, Page, Section, date } from '../../ui';

type Status = Schemas['TelegramStatus'];
type Intent = { kind: 'create' } | { kind: 'confirm'; value: Challenge } | { kind: 'challenge'; value: Challenge } | { kind: 'binding'; value: Binding };
async function ownStatus() { return client.request<Status>('/me/telegram'); }
export default function TelegramAccess() {
  const page = usePage(scope => client.request<Status>('/me/telegram', { scope }));
  const [intent, setIntent] = useState<Intent>(), [issued, setIssued] = useState<Challenge>(), [copyError, setCopyError] = useState<unknown>();
  useDidHide(() => { setIntent(undefined); setIssued(undefined); setCopyError(undefined); });
  const status = page.result?.value, me = page.result?.me;
  async function reload() { setIntent(undefined); setIssued(undefined); setCopyError(undefined); await page.reload(); }
  async function copied(text: string) { try { await copyTelegramProof(text); } catch (e) { setCopyError(e); } }
  return <Page title='Telegram 接入' eyebrow='IDENTITY / TELEGRAM'>
    <Notice error={page.error ?? copyError} />{page.loading ? <Busy /> : status && me && <>
      <Section title='接入与通知状态'><View className='cp-pad'>
        <Text className='cp-paragraph'>{status.ingress_configured ? '已配置Bot接收入口；完成本人绑定后，仍须符合指定群与话题的接收范围。' : '尚未配置Bot接收入口，请联系管理员。'}</Text>
        <Text className='cp-notice cp-paragraph'>{status.delivery_available ? '通知进程近期在线，实际送达请核对本人请求的投递回执。' : '当前未确认群通知进程在线；不表示消息已送达。'} 接收请求不代表已执行或获准群披露；业务正文仍需登录门户查阅。</Text>
        <Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/telegram/index' })}>查看本人TG请求与通知回执</Action>
      </View></Section>
      {intent ? <BindingAction key={intent.kind + ('value' in intent ? intent.value.id + ':' + intent.value.version : '')}
        intent={intent} bots={status.bots} onClose={() => setIntent(undefined)} onReload={() => void reload()}
        onCreated={async value => { setIssued(value); setIntent(undefined); await page.reload(); }} onSaved={() => void reload()} /> : <>
        <Section title='本人数字身份绑定'><View className='cp-pad'>
          <Text className='cp-muted cp-paragraph'>只信任私聊证明的数字身份。请在发起流程的原登录内核对并确认；昵称和群成员身份不能代替本人绑定。</Text>
          {status.bindings.length ? status.bindings.map(binding => <View className='cp-editor-row' key={binding.id}>
            <Fact label='Bot'><Text>@{status.bots.find(bot => bot.bot_id === binding.bot_id)?.username ?? binding.bot_id}</Text></Fact>
            <Fact label='本人数字身份'><Text selectable>{binding.telegram_user_id}</Text></Fact><Fact label='绑定时间'><Text>{date(binding.created_at)}</Text></Fact>
            <Action secondary onClick={() => setIntent({ kind: 'binding', value: binding })}>撤销此本人绑定</Action>
          </View>) : <Empty>尚无有效本人绑定。</Empty>}
          <Action disabled={!status.ingress_configured || !status.bots.length} onClick={() => { setIssued(undefined); setIntent({ kind: 'create' }); }}>发起本人TG绑定</Action>
        </View></Section>
        <Section title='待完成的绑定流程'><View className='cp-pad'>
          {!status.challenges.length && <Empty>没有待完成的本人绑定。</Empty>}
          {status.challenges.map(challenge => <View className='cp-editor-row' key={challenge.id}>
            <Fact label='所选Bot'><Text>@{status.bots.find(bot => bot.bot_id === challenge.bot_id)?.username ?? challenge.bot_id}</Text></Fact>
            <Fact label='流程到期'><Text>{date(challenge.expires_at)}</Text></Fact>
            {challenge.telegram_user_id ? <><Fact label='私聊证明的数字身份'><Text selectable>{challenge.telegram_user_id}</Text></Fact>
              <Action onClick={() => { setIssued(undefined); setIntent({ kind: 'confirm', value: challenge }); }}>核对并确认本人数字身份</Action></>
              : issued?.id === challenge.id && issued.start_token && issued.start_url ? <View className='cp-secret'>
                <Text className='cp-notice cp-paragraph'>仅在所选Bot私聊使用，勿发送到群。链接只保留在本页内存；离开或刷新后不再显示。</Text>
                <Text selectable className='cp-code'>/start {issued.start_token}</Text>
                <Action secondary onClick={() => void copied(issued.start_url!)}>复制本人私聊链接</Action>
                <Action secondary onClick={() => void copied('/start ' + issued.start_token)}>复制本人私聊命令</Action>
              </View> : <Text className='cp-muted cp-paragraph'>请在所选Bot私聊完成证明。链接只在发起时显示；已关闭或刷新可取消后重新发起。</Text>}
            <Action secondary onClick={() => { setIssued(undefined); setIntent({ kind: 'challenge', value: challenge }); }}>取消此次绑定流程</Action>
          </View>)}
        </View></Section>
        {me.actions.includes('policy.manage') && <TelegramChannels bots={status.bots} authorization={me.authorization_version} />}
      </>}
    </>}
    <Action secondary loading={page.loading} onClick={() => void reload()}>重新读取TG权限与本人状态</Action>
  </Page>;
}

function BindingAction({ intent, bots, onClose, onReload, onCreated, onSaved }: { intent: Intent; bots: Schemas['TelegramBot'][];
  onClose(): void; onReload(): void; onCreated(value: Challenge): void; onSaved(): void }) {
  const [botID, setBotID] = useState(''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [checked, setChecked] = useState(false);
  const target = 'value' in intent ? intent.value : undefined, selected = bots.find(bot => bot.bot_id === botID);
  const titles = { create: '发起本人TG绑定', confirm: '确认本人数字身份', challenge: '取消绑定流程', binding: '撤销本人TG绑定' };
  const command = useCommand<Challenge | Binding>(value => { if (intent.kind === 'create') onCreated(value as Challenge); else onSaved(); },
    'telegram-identity:' + intent.kind + ':' + (target ? target.id + ':' + target.version : 'new'), async value => {
      if (intent.kind === 'create') return !!selected && challengeReceipt(value, selected) && (await ownStatus()).challenges.some(c => c.id === value.id && c.bot_id === value.bot_id);
      if (intent.kind === 'confirm') return bindingReceipt(value, intent.value) && (await ownStatus()).bindings.some(b => b.id === value.id && b.version === value.version && bindingReceipt(b, intent.value));
      if (!endedReceipt(value, intent.value, intent.kind)) return false;
      const status = await ownStatus(); return !(intent.kind === 'challenge' ? status.challenges : status.bindings).some(v => v.id === intent.value.id);
    });
  const disabled = command.busy || command.blocked;
  async function submit() {
    let explanation: string; try { explanation = telegramReason(reason); } catch { return; }
    if (!checked || (intent.kind === 'create' || intent.kind === 'confirm') && !fresh) return;
    if (intent.kind === 'create') { if (selected) await command.submit('/me/telegram/challenges', { bot_id: selected.bot_id, reason: explanation }); }
    else if (intent.kind === 'confirm') await command.submit('/me/telegram/challenges/' + intent.value.id + '/confirm', { expected_version: intent.value.version, telegram_user_id: intent.value.telegram_user_id, reason: explanation });
    else await command.submit('/me/telegram/' + (intent.kind === 'challenge' ? 'challenges/' + intent.value.id + '/cancel' : 'bindings/' + intent.value.id + '/revoke'), { expected_version: intent.value.version, reason: explanation });
  }
  return <Section title={titles[intent.kind]}><View className='cp-pad'><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    {intent.kind === 'create' ? <SelectField label='选择绑定Bot' value={botID} options={[['', '请明确选择Bot'], ...bots.map(b => [b.bot_id, '@' + b.username] as const)]} disabled={disabled} onChange={value => { setBotID(value); setChecked(false); }} />
      : <><Fact label='准确Bot'><Text>{target!.bot_id}</Text></Fact><Fact label='准确流程版本'><Text>{target!.version}</Text></Fact>{target!.telegram_user_id && <Fact label='本人数字身份'><Text selectable>{target!.telegram_user_id}</Text></Fact>}</>}
    <Text className='cp-notice cp-paragraph'>{intent.kind === 'binding' ? '撤销后该身份停止接收新请求；尚未执行的请求一并撤回，既有记录仍保留。'
      : intent.kind === 'challenge' ? '取消后本次绑定码与私聊证明失效，不会撤销其他有效绑定。'
      : intent.kind === 'create' ? '流程十分钟内有效；同Bot旧流程会取消。生成链接不等于已经绑定。' : '请核对数字身份确实属于本人。在原登录近期两步验证后明确确认。'}</Text>
    <TextField name='tg-identity-reason' label='身份操作原因' value={reason} onChange={setReason} disabled={disabled} />
    <Check checked={checked} disabled={disabled} onChange={setChecked}>{intent.kind === 'create' ? '我将仅在所选Bot私聊证明本人身份' : intent.kind === 'confirm' ? '我已核对该数字身份属于本人' : '我已核对准确对象与此次取消或撤销的影响'}</Check>
    {(intent.kind === 'create' || intent.kind === 'confirm') && <MFAConfirmation onFresh={setFresh} disabled={disabled} />}
    <Action disabled={disabled || !reason.trim() || !checked || (intent.kind === 'create' || intent.kind === 'confirm') && !fresh || intent.kind === 'create' && !selected} loading={command.busy} onClick={() => void submit()}>
      {intent.kind === 'create' ? '生成本人私聊绑定链接' : intent.kind === 'confirm' ? '确认绑定这个数字身份' : intent.kind === 'challenge' ? '确认取消此次绑定' : '确认撤销此本人绑定'}</Action>
    {command.blocked && <Action secondary onClick={onReload}>返回本人状态核对结果</Action>}
    <Action secondary disabled={command.busy} onClick={onClose}>关闭身份操作表单</Action>
  </View></Section>;
}
