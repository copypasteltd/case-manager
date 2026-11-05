import {test} from 'node:test';
import {batch157Cases} from '../../../packages/contracts/test/batch157-cases';
for(const [name,check] of batch157Cases)test(name,check);
