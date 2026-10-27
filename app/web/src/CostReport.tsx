import type { Me } from './api';
import FinancialReports from './FinancialReports';
export default function CostReport({ me }: { me: Me }) { return <FinancialReports me={me} initialKind="incurred" />; }
