export interface Post {
  id: string;
  author: string;
  timestamp: number;
  content: string;
  mediaCids: string[];
  replyTo?: string;
  repostOf?: string;
  tags: string[];
}

export interface Reaction {
  id: string;
  author: string;
  target: string;
  reactionType: string;
  timestamp: number;
}

export interface Profile {
  author: string;
  displayName: string;
  bio: string;
  avatarCid?: string;
  bannerCid?: string;
  website?: string;
  version: number;
}

export interface DirectMessage {
  id: string;
  author: string;
  recipient: string;
  timestamp: number;
}

export interface Notification {
  id: number;
  type: string;
  actor: string;
  targetCid?: string;
  relatedCid?: string;
  timestamp: number;
  read: boolean;
}

export interface FeedEntry {
  post: Post;
  authorName: string;
  likeCount: number;
  replyCount: number;
  repostCount: number;
  isLiked: boolean;
  isReposted: boolean;
}

export interface NodeStatus {
  peers: number;
  bandwidth: { totalIn: number; totalOut: number };
  storage: { usedGB: number; maxGB: number };
  uptime: number;
}

export interface ConversationSummary {
  pubkey: string;
  displayName: string;
  lastMessage: string;
  timestamp: number;
  unread: number;
}

export interface WSEvent {
  type: string;
  data: unknown;
}
