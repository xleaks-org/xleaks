import type {
  FeedEntry,
  Post,
  Reaction,
  Profile,
  Notification,
  ConversationSummary,
  DirectMessage,
  NodeStatus,
} from './types';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || '/api';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`API error ${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

// Feed
export async function getFeed(
  cursor?: string,
  limit = 20
): Promise<{ entries: FeedEntry[]; nextCursor?: string }> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (cursor) params.set('cursor', cursor);
  return request(`/feed?${params}`);
}

// Posts
export async function createPost(data: {
  content: string;
  mediaCids?: string[];
  replyTo?: string;
  tags?: string[];
}): Promise<Post> {
  return request('/posts', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function getPost(id: string): Promise<FeedEntry> {
  return request(`/posts/${id}`);
}

export async function getThread(
  id: string
): Promise<{ root: FeedEntry; replies: FeedEntry[] }> {
  return request(`/posts/${id}/thread`);
}

// Reactions
export async function createReaction(data: {
  target: string;
  reactionType: string;
}): Promise<Reaction> {
  return request('/reactions', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// Follow
export async function followUser(pubkey: string): Promise<void> {
  await request(`/follow/${pubkey}`, { method: 'POST' });
}

export async function unfollowUser(pubkey: string): Promise<void> {
  await request(`/follow/${pubkey}`, { method: 'DELETE' });
}

// Profiles
export async function getProfile(pubkey: string): Promise<Profile> {
  return request(`/profiles/${pubkey}`);
}

export async function updateProfile(
  data: Partial<Omit<Profile, 'author' | 'version'>>
): Promise<Profile> {
  return request('/profiles', {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

// Notifications
export async function getNotifications(): Promise<Notification[]> {
  return request('/notifications');
}

export async function markNotificationsRead(): Promise<void> {
  await request('/notifications/read', { method: 'POST' });
}

export async function getUnreadCount(): Promise<{ count: number }> {
  return request('/notifications/unread');
}

// Direct Messages
export async function getConversations(): Promise<ConversationSummary[]> {
  return request('/messages');
}

export async function getConversation(
  pubkey: string
): Promise<DirectMessage[]> {
  return request(`/messages/${pubkey}`);
}

export async function sendDM(data: {
  recipient: string;
  content: string;
}): Promise<DirectMessage> {
  return request('/messages', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// Media
export async function uploadMedia(file: File): Promise<{ cid: string }> {
  const formData = new FormData();
  formData.append('file', file);
  const res = await fetch(`${API_BASE}/media`, {
    method: 'POST',
    body: formData,
  });
  if (!res.ok) {
    throw new Error(`Upload failed: ${res.statusText}`);
  }
  return res.json();
}

// Node
export async function getNodeStatus(): Promise<NodeStatus> {
  return request('/node/status');
}

// Identity
export async function createIdentity(data: {
  passphrase: string;
}): Promise<{ pubkey: string; seedPhrase: string; mnemonic: string; address: string }> {
  return request('/identity/create', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function importIdentity(data: {
  seedPhrase: string;
  passphrase: string;
}): Promise<{ pubkey: string; address: string }> {
  return request('/identity/import', {
    method: 'POST',
    body: JSON.stringify({ mnemonic: data.seedPhrase, passphrase: data.passphrase }),
  });
}

export async function unlockIdentity(data: {
  passphrase: string;
}): Promise<{ pubkey: string }> {
  return request('/identity/unlock', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function getActiveIdentity(): Promise<{
  active: boolean;
  locked?: boolean;
  needsOnboarding?: boolean;
  pubkey?: string;
  address?: string;
  displayName?: string;
} | null> {
  try {
    return await request('/identity/active');
  } catch {
    return null;
  }
}

// Search
export async function searchPosts(
  query: string
): Promise<{ entries: FeedEntry[] }> {
  return request(`/search/posts?q=${encodeURIComponent(query)}`);
}

export async function searchUsers(
  query: string
): Promise<{ profiles: Profile[] }> {
  return request(`/search/users?q=${encodeURIComponent(query)}`);
}

// Trending
export async function getTrending(): Promise<{
  tags: { tag: string; count: number }[];
  posts: FeedEntry[];
}> {
  return request('/trending');
}

// User posts
export async function getUserPosts(
  pubkey: string,
  tab: 'posts' | 'replies' | 'media' | 'likes' = 'posts'
): Promise<{ entries: FeedEntry[] }> {
  return request(`/profiles/${pubkey}/${tab}`);
}
