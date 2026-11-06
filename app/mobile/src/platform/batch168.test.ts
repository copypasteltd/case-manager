import {test} from 'node:test';
import assert from 'node:assert/strict';
import {batch168Cases} from '../../../packages/contracts/test/batch168-cases';
import {MiniUploadSources,MINI_UPLOAD_BYTES} from './mini-upload-source';
for(const [name,check] of batch168Cases)test(name,check);
test('mini originals are bounded, fully hashed, reselected exactly and cleared on owner loss',async()=>{
 let bytes=new Uint8Array([1,2,3]),serial=0,size=3;const source=new MiniUploadSources({key:()=>String(++serial),pick:async()=>({path:'temporary',name:'receipt.bin',size,media:'application/octet-stream'}),size:async()=>size,read:async()=>bytes.slice().buffer,put:()=>{throw new Error('unexpected transfer');}});
 await source.scope('owner');const item=await source.select();assert.match(item.fingerprint,/^source-sha256-v2:[a-f0-9]{64}$/);await source.prepare(item.handle,{folder_id:'fixed-folder',name:'receipt.bin'});assert.equal((await source.list())[0].create_key.length>0,true);bytes=new Uint8Array([1,9,3]);await assert.rejects(source.verifySource(item.handle));await assert.rejects(source.select(item.handle));size=MINI_UPLOAD_BYTES+1;await assert.rejects(source.select());source.clear();assert.deepEqual(await source.list(),[]);await assert.rejects(source.select());
});
