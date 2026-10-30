import { describe, it } from 'vitest';
import { officeAuthorCases } from '../../packages/contracts/test/office-author-cases';
describe('Office authoring business inputs',()=>{for(const [name,run] of officeAuthorCases)it(name,run);});
