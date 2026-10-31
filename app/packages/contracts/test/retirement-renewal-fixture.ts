import { retirementID as id, retirementPlan } from './object-retirement-fixture';
import type { RetirementPlan } from '../src/object-retirement';
export function stoppedRetirement(): RetirementPlan {
 const p=retirementPlan();
 return {...p,state:'approved',version:2,approved_at:p.ready_after,approval_reason:'original observed human decision',items:[{...p.items[0],state:'succeeded',version:3,confirmed_at:p.ready_after},{...p.items[1],state:'blocked',version:4,last_error:'AUTHORIZATION_CHANGED'}]};
}
export function renewedRetirement(): RetirementPlan {
 const p=stoppedRetirement(), item=p.items[1];
 return {...p,version:3,authorization:{id:id(20),sequence:1,plan_version:3,snapshot_sha256:p.snapshot_sha256,reason:'new human authority for remaining source',authorized_at:p.expires_at,authorized_by:id(21),channel:'web',items:[{item_id:item.id,previous_version:item.version,generation:item.generation+1}]},items:[p.items[0],{...item,state:'queued',version:item.version+1,generation:item.generation+1,last_error:null}]};
}
