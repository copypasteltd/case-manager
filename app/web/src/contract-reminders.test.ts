import { describe,it } from 'vitest';
import { reminderCases } from '../../packages/contracts/test/contract-reminders-cases';
describe('explicit contract reminder policies',()=>{for(const [name,run] of reminderCases)it(name,run);});
