import test from 'node:test';
import { contractFinanceCases } from '../../../packages/contracts/test/contract-finance-cases';
for (const [name, run] of contractFinanceCases) test(name, run);
