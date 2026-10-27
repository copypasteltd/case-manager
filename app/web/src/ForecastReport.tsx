import type { Me } from './api';
import FinancialReports from './FinancialReports';
export default function ForecastReport({ me }: { me: Me }) { return <FinancialReports me={me} initialKind="forecast" />; }
