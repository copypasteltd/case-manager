import { Bell } from 'lucide-react';
import { NavLink } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { notificationCount } from '@copypaste/contracts/notifications';
import { api, type Me, type Schemas } from './api';
export function useNotificationSummary(me: Me) {
  return useQuery({ queryKey: ['notifications-summary', me.user.id, me.authorization_version], queryFn: ({ signal }) => api<Schemas['NotificationSummary']>('/notifications/summary', { signal }), refetchInterval: 30000, gcTime: 0 });
}
export function NotificationIndicator({ me }: { me: Me }) {
  const query = useNotificationSummary(me), value = query.error ? undefined : query.data;
  return <NavLink to="/notifications"><Bell size={19} /><span>通知中心</span>{value && <span className="badge neutral" aria-label={'未读 ' + notificationCount(value.unread_count, value.count_complete)}>{notificationCount(value.unread_count, value.count_complete)}</span>}</NavLink>;
}
