import type { components } from './api';
type S = components['schemas'];
export type Invitation = S['InvitationReceipt'];
const id = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;

export function reissueInvitationInput(receipt: Invitation, reason: string): S['InvitationReissueInput'] {
  if (receipt.user.status !== 'invited' || !id.test(receipt.user.id) || !id.test(receipt.invitation_id) || !Number.isSafeInteger(receipt.user.version)) throw new Error('请刷新待激活成员的邀请。');
  reason = reason.trim();
  if (!reason || [...reason].length > 200 || /[\p{Cc}\p{Cf}]/u.test(reason)) throw new Error('请输入200字以内的重发原因。');
  return { expected_version: receipt.user.version, invitation_id: receipt.invitation_id, reason };
}

export function checkedReissuedInvitation(before: Invitation, next: Invitation): Invitation {
  if (!next || next.user?.id !== before.user.id || next.user.status !== 'invited' || next.user.version !== before.user.version + 1 ||
      !id.test(next.invitation_id) || next.invitation_id === before.invitation_id || next.replaces_id !== before.invitation_id ||
      next.revoked_at || next.consumed_at || !/^[A-Za-z0-9_-]{43}$/.test(next.activation_token ?? '') || !Number.isFinite(Date.parse(next.expires_at))) throw new Error('重发结果尚未核对，请刷新成员和当前邀请。');
  return next;
}
