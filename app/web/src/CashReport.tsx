import type { Me } from './api';
import FinancialReports from './FinancialReports';
export default function CashReport({ me }: { me: Me }) { return <FinancialReports me={me} initialKind="cash" />; }
