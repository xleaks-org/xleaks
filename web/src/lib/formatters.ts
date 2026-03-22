/**
 * Format a Unix timestamp (seconds) to relative time ("2m ago", "3h ago", "5d ago")
 */
export function formatRelativeTime(timestampMs: number): string {
  const now = Date.now();
  const diff = now - timestampMs;

  if (diff < 0) return 'just now';

  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;

  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;

  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;

  const years = Math.floor(months / 12);
  return `${years}y ago`;
}

/**
 * Truncate a hex pubkey to short form: "abc123...def456"
 */
export function formatPubkey(pubkey: string, chars = 6): string {
  if (pubkey.length <= chars * 2 + 3) return pubkey;
  return `${pubkey.slice(0, chars)}...${pubkey.slice(-chars)}`;
}

/**
 * Format large numbers compactly: 1234 -> "1.2K", 1234567 -> "1.2M"
 */
export function formatCount(count: number): string {
  if (count < 1000) return String(count);
  if (count < 1_000_000) {
    const val = count / 1000;
    return `${val >= 10 ? Math.floor(val) : val.toFixed(1)}K`;
  }
  if (count < 1_000_000_000) {
    const val = count / 1_000_000;
    return `${val >= 10 ? Math.floor(val) : val.toFixed(1)}M`;
  }
  const val = count / 1_000_000_000;
  return `${val >= 10 ? Math.floor(val) : val.toFixed(1)}B`;
}

/**
 * Format bytes to human readable: 1024 -> "1 KB", 1048576 -> "1 MB"
 */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

/**
 * Format duration in seconds to "2h 30m" or "5d 12h"
 */
export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.floor(seconds)}s`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  if (hours < 24) {
    return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
  }

  const days = Math.floor(hours / 24);
  const remainingHours = hours % 24;
  return remainingHours > 0 ? `${days}d ${remainingHours}h` : `${days}d`;
}
