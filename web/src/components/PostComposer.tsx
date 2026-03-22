'use client';

import { useState, useRef, useCallback } from 'react';
import { createPost, uploadMedia } from '@/lib/api';

const MAX_CHARS = 5000;

export default function PostComposer({
  replyTo,
  onPostCreated,
}: {
  replyTo?: string;
  onPostCreated?: () => void;
}) {
  const [content, setContent] = useState('');
  const [posting, setPosting] = useState(false);
  const [mediaCids, setMediaCids] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const charCount = content.length;
  const isOverLimit = charCount > MAX_CHARS;
  const canPost = content.trim().length > 0 && !isOverLimit && !posting;

  const handleSubmit = useCallback(async () => {
    if (!canPost) return;
    setPosting(true);
    try {
      // Extract hashtags from content
      const tags = Array.from(
        content.matchAll(/#(\w+)/g),
        (m) => m[1]
      );
      await createPost({
        content,
        mediaCids: mediaCids.length > 0 ? mediaCids : undefined,
        replyTo,
        tags: tags.length > 0 ? tags : undefined,
      });
      setContent('');
      setMediaCids([]);
      onPostCreated?.();
    } catch {
      // Handle error
    } finally {
      setPosting(false);
    }
  }, [canPost, content, mediaCids, replyTo, onPostCreated]);

  const handleFileUpload = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    try {
      const result = await uploadMedia(file);
      setMediaCids((prev) => [...prev, result.cid]);
    } catch {
      // Handle error
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  }, []);

  return (
    <div className="border-b border-gray-800 px-4 py-3">
      <div className="flex gap-3">
        {/* Avatar */}
        <div className="shrink-0 w-10 h-10 rounded-full bg-gray-700 flex items-center justify-center text-sm font-bold text-white">
          ?
        </div>

        {/* Input area */}
        <div className="flex-1">
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder={replyTo ? 'Post your reply...' : "What's happening?"}
            className="w-full bg-transparent text-white text-lg placeholder-gray-500 resize-none outline-none min-h-[80px]"
            rows={3}
          />

          {/* Media previews */}
          {mediaCids.length > 0 && (
            <div className="flex flex-wrap gap-2 mt-2">
              {mediaCids.map((cid) => (
                <div
                  key={cid}
                  className="relative bg-gray-800 rounded-lg px-3 py-1 text-xs text-gray-400 flex items-center gap-2"
                >
                  <span>Media: {cid.slice(0, 8)}...</span>
                  <button
                    onClick={() =>
                      setMediaCids((prev) => prev.filter((c) => c !== cid))
                    }
                    className="text-gray-500 hover:text-white"
                  >
                    x
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Bottom bar */}
          <div className="flex items-center justify-between mt-3 pt-3 border-t border-gray-800">
            <div className="flex items-center gap-2">
              {/* Media upload */}
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*,video/*"
                className="hidden"
                onChange={handleFileUpload}
              />
              <button
                onClick={() => fileInputRef.current?.click()}
                disabled={uploading}
                className="text-blue-500 hover:text-blue-400 p-1.5 rounded-full hover:bg-blue-500/10 transition-colors disabled:opacity-50"
                aria-label="Upload media"
              >
                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909M3.75 21h16.5A2.25 2.25 0 0022.5 18.75V5.25A2.25 2.25 0 0020.25 3H3.75A2.25 2.25 0 001.5 5.25v13.5A2.25 2.25 0 003.75 21z" />
                </svg>
              </button>
              {uploading && (
                <span className="text-xs text-gray-400">Uploading...</span>
              )}
            </div>

            <div className="flex items-center gap-3">
              {/* Character counter */}
              {charCount > 0 && (
                <span
                  className={`text-sm ${
                    isOverLimit
                      ? 'text-red-500'
                      : charCount > MAX_CHARS * 0.9
                        ? 'text-yellow-500'
                        : 'text-gray-500'
                  }`}
                >
                  {charCount}/{MAX_CHARS}
                </span>
              )}

              {/* Post button */}
              <button
                onClick={handleSubmit}
                disabled={!canPost}
                className="bg-blue-500 hover:bg-blue-600 disabled:opacity-50 disabled:hover:bg-blue-500 text-white font-bold px-5 py-1.5 rounded-full transition-colors"
              >
                {posting ? 'Posting...' : 'Post'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
