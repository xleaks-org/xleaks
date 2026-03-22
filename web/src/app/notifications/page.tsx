'use client';

import { useNotifications } from '@/hooks/useNotifications';

function getNotificationIcon(type: string) {
  switch (type) {
    case 'like':
      return (
        <div className="w-8 h-8 rounded-full bg-pink-500/20 flex items-center justify-center">
          <svg className="w-4 h-4 text-pink-500" viewBox="0 0 24 24" fill="currentColor">
            <path d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z" />
          </svg>
        </div>
      );
    case 'repost':
      return (
        <div className="w-8 h-8 rounded-full bg-green-500/20 flex items-center justify-center">
          <svg className="w-4 h-4 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 12c0-1.232-.046-2.453-.138-3.662a4.006 4.006 0 00-3.7-3.7 48.678 48.678 0 00-7.324 0 4.006 4.006 0 00-3.7 3.7c-.017.22-.032.441-.046.662M19.5 12l3-3m-3 3l-3-3m-12 3c0 1.232.046 2.453.138 3.662a4.006 4.006 0 003.7 3.7 48.656 48.656 0 007.324 0 4.006 4.006 0 003.7-3.7c.017-.22.032-.441.046-.662M4.5 12l3 3m-3-3l-3 3" />
          </svg>
        </div>
      );
    case 'reply':
      return (
        <div className="w-8 h-8 rounded-full bg-blue-500/20 flex items-center justify-center">
          <svg className="w-4 h-4 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 20.25c4.97 0 9-3.694 9-8.25s-4.03-8.25-9-8.25S3 7.444 3 12c0 2.104.859 4.023 2.273 5.48.432.447.74 1.04.586 1.641a4.483 4.483 0 01-.923 1.785A5.969 5.969 0 006 21c1.282 0 2.47-.402 3.445-1.087.81.22 1.668.337 2.555.337z" />
          </svg>
        </div>
      );
    case 'follow':
      return (
        <div className="w-8 h-8 rounded-full bg-purple-500/20 flex items-center justify-center">
          <svg className="w-4 h-4 text-purple-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
          </svg>
        </div>
      );
    default:
      return (
        <div className="w-8 h-8 rounded-full bg-gray-700 flex items-center justify-center">
          <svg className="w-4 h-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
          </svg>
        </div>
      );
  }
}

function getNotificationText(type: string, actor: string): string {
  const name = actor.length > 12 ? `${actor.slice(0, 8)}...` : actor;
  switch (type) {
    case 'like':
      return `${name} liked your post`;
    case 'repost':
      return `${name} reposted your post`;
    case 'reply':
      return `${name} replied to your post`;
    case 'follow':
      return `${name} followed you`;
    case 'mention':
      return `${name} mentioned you`;
    default:
      return `${name} interacted with your content`;
  }
}

function formatTime(timestamp: number): string {
  const now = Date.now() / 1000;
  const diff = now - timestamp;
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return new Date(timestamp * 1000).toLocaleDateString();
}

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
            <div
              key={notif.id}
              className={`flex items-start gap-3 px-4 py-3 border-b border-gray-800 hover:bg-gray-900/50 transition-colors ${
                !notif.read ? 'bg-blue-500/5' : ''
              }`}
            >
              {getNotificationIcon(notif.type)}
              <div className="flex-1 min-w-0">
                <p className="text-sm text-white">
                  {getNotificationText(notif.type, notif.actor)}
                </p>
                <p className="text-xs text-gray-500 mt-0.5">
                  {formatTime(notif.timestamp)}
                </p>
              </div>
              {!notif.read && (
                <div className="w-2 h-2 rounded-full bg-blue-500 mt-2 shrink-0" />
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
