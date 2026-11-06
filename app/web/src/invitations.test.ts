import { describe, it, expect } from 'vitest';
import { checkedReissuedInvitation, reissueInvitationInput, type Invitation } from '@copypaste/contracts/invitations';
const id='11111111-1111-4111-8111-111111111111', old='22222222-2222-4222-8222-222222222222', fresh='33333333-3333-4333-8333-333333333333';
const before:Invitation={user:{id,version:2,status:'invited',display_name:'成员'},invitation_id:old,expires_at:'2026-01-01T00:00:00Z'};
const after:Invitation={...before,user:{...before.user,version:3},invitation_id:fresh,replaces_id:old,activation_token:'a'.repeat(43),expires_at:'2027-01-01T00:00:00Z'};
describe('fixed invitation renewal',()=>{
  it('binds the human review to one invited user and exact prior invitation',()=>expect(reissueInvitationInput(before,' 重新交付 ')).toEqual({expected_version:2,invitation_id:old,reason:'重新交付'}));
  it('requires an unambiguous same-user next revision receipt',()=>{expect(checkedReissuedInvitation(before,after)).toBe(after);for(const patch of [{replaces_id:fresh},{invitation_id:old},{revoked_at:after.expires_at},{activation_token:undefined},{user:{...after.user,id:fresh}},{user:{...after.user,version:4}},{user:{...after.user,status:'active' as const}}])expect(()=>checkedReissuedInvitation(before,{...after,...patch})).toThrow();});
  it('does not turn activation or an empty explanation into renewal',()=>{expect(()=>reissueInvitationInput({...before,user:{...before.user,status:'active'}},'reason')).toThrow();for(const reason of ['','a'.repeat(201),'a\u0000b'])expect(()=>reissueInvitationInput(before,reason)).toThrow();});
});
