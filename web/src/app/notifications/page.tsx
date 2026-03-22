'use client';

import { useNotifications } from '@/hooks/useNotifications';
import NotificationItem from '@/components/NotificationItem';

export default function NotificationsPage() {
  const { notifications, loading, markRead } = useNotifications();

  return (
    <div>
      {/* Header */}
      <header className="sticky top-0 z-10 bg-gray-950/80 backdrop-blur-md border-b border-gray-800 px-4 py-3 flex items-center justify-between">
        <h1 className="text-xl font-bold text-white">Notifications</h1>
        <button
          onClick={markRead}
          className="text-sm text-blue-500 hover:text-blue-400 transition-colors"
        >
          Mark all read
        </button>
      </header>

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : notifications.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          <svg className="w-12 h-12 mx-auto mb-3 text-gray-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
          </svg>
          <p className="text-lg">No notifications</p>
          <p className="text-sm mt-1">
            When someone interacts with your content, you will see it here
          </p>
        </div>
      ) : (
        <div>
          {notifications.map((notif) => (
            <NotificationItem key={notif.id} notification={notif} />
          ))}
        </div>
      )}
    </div>
  );
}
