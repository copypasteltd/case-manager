import {test} from 'node:test';
import {batch159Cases} from '../../../packages/contracts/test/batch159-cases';
for(const[name,run]of batch159Cases)test(name,run);
