import test from 'node:test';
import { paymentFeesCases } from '../../../packages/contracts/test/payment-fees-cases';
for (const [name, run] of paymentFeesCases) test(name, run);
