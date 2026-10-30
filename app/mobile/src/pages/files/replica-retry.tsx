import { Text } from '@tarojs/components';
import { useState } from 'react';
import { checkedReplicaRetry, replicaRetryInput } from '@copypaste/contracts/file-replica';
import { CommandNotice, useCommand } from '../../platform/command';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Check, Notice, Section } from '../../ui';
import { TextField } from '../business/controls';

export function ReplicaRetry({ replica, fileID, authorization, close, committed }: {
  replica: Schemas['FileReplica']; fileID: string; authorization: string | number; close(): void; committed(value: Schemas['FileReplica']): void;
}) {
  const [base] = useState(replica), [reason, setReason] = useState(''), [ack, setAck] = useState(false), [error, setError] = useState<unknown>();
  const command = useCommand<Schemas['FileReplica']>(committed, ['replica-retry', fileID, base.id, base.version, authorization].join(':'), value => { checkedReplicaRetry(value, base); return true; });
  const locked = command.busy || command.blocked;
  function submit() {
    if (!ack) return;
    try { const body = replicaRetryInput(base, reason); setError(undefined); void command.submit('/file-replicas/' + base.id + '/retry', body); }
    catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对副本资料。')); }
  }
  return <Section title='重新安排副本复制'><Text className='cp-paragraph'>{base.provider === 'oss' ? 'OSS 原件' : 'R2 副本'} · 副本修订 {base.version}</Text><Text className='cp-muted cp-paragraph'>排队成功后仍需等待复制及完整校验。服务器会重新核对权限、版本和来源副本。</Text><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} /><TextField name='replica-retry-reason' label='重试原因' multiline value={reason} onChange={setReason} disabled={locked} /><Check checked={ack} disabled={locked} onChange={setAck}>我已核对这个副本及重试原因</Check><Action loading={command.busy} disabled={locked || !ack || !reason.trim()} onClick={submit}>安排复制</Action><Action secondary disabled={command.busy} onClick={close}>返回存储记录</Action></Section>;
}
