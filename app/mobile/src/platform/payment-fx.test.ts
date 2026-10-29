import test from 'node:test';
import { paymentFXCases } from '../../../packages/contracts/test/payment-fx-cases';
for (const [name, run] of paymentFXCases) test(name, run);
