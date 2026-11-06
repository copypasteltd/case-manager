import {test} from 'node:test';
import {batch160Cases} from '../../../packages/contracts/test/batch160-cases';
for(const[name,run]of batch160Cases)test(name,run);
