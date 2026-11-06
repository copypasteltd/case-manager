import { test,expect } from 'vitest';
const assert={equal:(a:unknown,b:unknown)=>expect(a).toBe(b)};
import { telegramGrantAccepted, grantCancellationAccepted, untriggeredGrant, telegramProjectTools } from '@copypaste/contracts/telegram-grants';
import type { components } from '@copypaste/contracts';
type S=components['schemas'];
const id='11111111-1111-4111-8111-111111111111',run='22222222-2222-4222-8222-222222222222',conv='33333333-3333-4333-8333-333333333333';
const input={channel_id:id,channel_version:2,binding_id:id,binding_version:3,reason:'Exact reviewed instruction',run:{expected_version:1,context_security_version:1,profile_id:id,client_message_id:id,message:'Only fixed change',mutation:{tool:'create_task',arguments:{id,expected_version:1,reason:"Only this fixed change"}}}} as S['TelegramWriteGrantInput'];
const grant={id,run_id:run,conversation_id:conv,channel_id:id,channel_version:2,binding_id:id,binding_version:3,input_sha256:'a'.repeat(64),state:'pending_trigger',tool:'create_task',client_message_id:id,run_version:1,run_state:'queued',trigger:'/task project-update authorize:'+id,created_at:'2026-11-06T00:00:00Z',expires_at:'2026-11-06T00:10:00Z'} as S['TelegramWriteGrant'];
test('all sixteen exact project tools accept only their matching grant receipt',()=>{
 assert.equal(telegramProjectTools.length,16);
 for(const tool of telegramProjectTools){assert.equal(telegramGrantAccepted({...grant,tool},conv,{...input,run:{...input.run,mutation:{tool,arguments:{id,expected_version:1,reason:"Only this fixed change"}}}}),true);}
 for(const patch of [{tool:'update_task'},{client_message_id:run},{binding_version:2},{channel_version:1},{conversation_id:run},{run_version:2},{run_state:'running'},{state:'consumed'},{trigger:'/task project-update authorize:'+run},{expires_at:'2026-11-06T00:10:01Z'},{input_sha256:'bad'}]) assert.equal(telegramGrantAccepted({...grant,...patch} as S['TelegramWriteGrant'],conv,input),false);
});
test('ordinary cancellation receipt is bound to the selected untriggered run',()=>{
 assert.equal(untriggeredGrant(grant),true);assert.equal(untriggeredGrant({...grant,state:'invalidated'}),true);assert.equal(untriggeredGrant({...grant,state:'consumed'}),false);
 const stopped={id:run,conversation_id:conv,version:2,state:'cancelled',cancel_requested_at:'2026-11-06T00:01:00Z'} as S['Run'];
 assert.equal(grantCancellationAccepted(stopped,grant),true);assert.equal(grantCancellationAccepted({...stopped,id},grant),false);assert.equal(grantCancellationAccepted({...stopped,state:'succeeded'},grant),false);assert.equal(grantCancellationAccepted({...stopped,cancel_requested_at:null},grant),false);
});
