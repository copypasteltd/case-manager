import { describe, it } from 'vitest';
import { paymentFXCases } from '../../packages/contracts/test/payment-fx-cases';
describe('reviewed cross currency settlement', () => { for (const [name, run] of paymentFXCases) it(name, run); });
