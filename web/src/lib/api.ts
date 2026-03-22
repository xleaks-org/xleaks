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

let apiToken: string | null = null;

/**
 * Initializes the API token by reading from a cookie. If no cookie is found
 * the token is left empty. When the server has no token configured, requests
 * without an Authorization header are allowed through.
 */
async function initToken(): Promise<void> {
  if (apiToken !== null) return;

  // Try reading from cookie first.
  const cookieToken = document.cookie
    .split('; ')
    .find((c) => c.startsWith('xleaks_token='))
    ?.split('=')[1];
  if (cookieToken) {
    apiToken = cookieToken;
    return;
  }

  // No cookie found; leave as empty string (no auth header sent).
  apiToken = '';
}

function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (apiToken) {
    headers['Authorization'] = `Bearer ${apiToken}`;
  }
  return headers;
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  await initToken();

  const res = await fetch(`${API_BASE}${path}`, {
    headers: authHeaders(),
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
  return request(`/users/${pubkey}`);
}

export async function getOwnProfile(): Promise<Profile> {
  return request('/profile');
}

export async function updateProfile(
  data: Partial<Omit<Profile, 'author' | 'version'>>
): Promise<Profile> {
  return request('/profile', {
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
  return request('/notifications/unread-count');
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
  await initToken();

  const formData = new FormData();
  formData.append('file', file);

  const headers: Record<string, string> = {};
  if (apiToken) {
    headers['Authorization'] = `Bearer ${apiToken}`;
  }

  const res = await fetch(`${API_BASE}/media`, {
    method: 'POST',
    headers,
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

export async function getNodeConfig(): Promise<{
  maxStorageGB: number;
  bootstrapPeers: string[];
  relayEnabled: boolean;
}> {
  return request('/node/config');
}

export async function updateNodeConfig(data: {
  maxStorageGB?: number;
}): Promise<void> {
  await request('/node/config', {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

// Repost
export async function createRepost(postCid: string): Promise<Post> {
  return request('/repost', {
    method: 'POST',
    body: JSON.stringify({ post_cid: postCid }),
  });
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
