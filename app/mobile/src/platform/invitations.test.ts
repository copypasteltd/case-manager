import test from 'node:test';
import assert from 'node:assert/strict';
import { checkedReissuedInvitation, reissueInvitationInput, type Invitation } from '../../../packages/contracts/src/invitations';
const user='11111111-1111-4111-8111-111111111111', old='22222222-2222-4222-8222-222222222222', fresh='33333333-3333-4333-8333-333333333333';
const before:Invitation={user:{id:user,version:1,status:'invited',display_name:'成员'},invitation_id:old,expires_at:'2026-01-01T00:00:00Z'};
test('mobile renewal fixes the current invited identity and requires the exact replacement receipt',()=>{const input=reissueInvitationInput(before,'重新交付');assert.equal(input.invitation_id,old);const next:Invitation={...before,user:{...before.user,version:2},invitation_id:fresh,replaces_id:old,activation_token:'b'.repeat(43),expires_at:'2027-01-01T00:00:00Z'};assert.equal(checkedReissuedInvitation(before,next),next);assert.throws(()=>checkedReissuedInvitation(before,{...next,replaces_id:user}));assert.throws(()=>checkedReissuedInvitation(before,{...next,consumed_at:next.expires_at}));});
test('mobile renewal does not reset disabled users or silently rebase stale revisions',()=>{assert.throws(()=>reissueInvitationInput({...before,user:{...before.user,status:'disabled'}},'reason'));assert.throws(()=>reissueInvitationInput(before,'a'.repeat(201)));});
