import { it } from 'vitest';
import { documentTextCases } from '../../packages/contracts/test/document-text-cases';
for (const [name, check] of documentTextCases) it(name, check);
