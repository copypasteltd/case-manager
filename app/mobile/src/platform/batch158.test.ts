import {test} from 'node:test';
import {batch158Cases} from '../../../packages/contracts/test/batch158-cases';
for(const[name,run]of batch158Cases)test(name,run);
