import {describe,it} from 'vitest';
import {batch157Cases} from '../../packages/contracts/test/batch157-cases';
describe('batch 157 shared authorization and publication contracts',()=>{for(const [name,check] of batch157Cases)it(name,check);});
