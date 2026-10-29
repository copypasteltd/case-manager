import test from 'node:test';
import { reminderCases } from '../../../packages/contracts/test/contract-reminders-cases';
for(const [name,run] of reminderCases)test(name,run);
