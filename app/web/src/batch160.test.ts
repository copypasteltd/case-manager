import {describe,it} from 'vitest';
import {batch160Cases} from '../../packages/contracts/test/batch160-cases';
describe('batch160 complete code wave',()=>{for(const[name,run]of batch160Cases)it(name,run);});
