import test from 'node:test';
import { officeCreateCases } from '../../../packages/contracts/test/office-create-cases';
for (const [name, check] of officeCreateCases) test(name, check);
