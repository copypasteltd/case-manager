import test from 'node:test';
import { scopeTransferCases } from '../../../packages/contracts/test/scope-transfer-cases';
for (const [name, run] of scopeTransferCases) test(name, run);
