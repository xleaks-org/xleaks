'use client';

import { useState, useCallback } from 'react';

const MEDIA_BASE = 'http://localhost:7470/api/media';

function isVideoUrl(cid: string): boolean {
  // We can't know from CID alone, but we'll try to render as image first
  // and fall back. For simplicity, we treat all as images unless the element errors.
  return cid.endsWith('.mp4') || cid.endsWith('.webm');
}

export default function MediaViewer({ mediaCids }: { mediaCids: string[] }) {
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const [videoErrors, setVideoErrors] = useState<Set<string>>(new Set());

  const openLightbox = useCallback((index: number) => {
    setLightboxIndex(index);
  }, []);

  const closeLightbox = useCallback(() => {
    setLightboxIndex(null);
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (lightboxIndex === null) return;
      if (e.key === 'Escape') closeLightbox();
      if (e.key === 'ArrowRight' && lightboxIndex < mediaCids.length - 1) {
        setLightboxIndex(lightboxIndex + 1);
      }
      if (e.key === 'ArrowLeft' && lightboxIndex > 0) {
        setLightboxIndex(lightboxIndex - 1);
      }
    },
    [lightboxIndex, mediaCids.length, closeLightbox]
  );

  const markAsVideo = useCallback((cid: string) => {
    setVideoErrors((prev) => new Set(prev).add(cid));
  }, []);

  if (mediaCids.length === 0) return null;

  const gridClass =
    mediaCids.length === 1
      ? 'grid-cols-1'
      : mediaCids.length === 2
        ? 'grid-cols-2'
        : 'grid-cols-2';

  return (
    <>
      <div
        className={`grid ${gridClass} gap-1 mt-3 rounded-xl overflow-hidden border border-gray-800`}
      >
        {mediaCids.map((cid, index) => {
          const url = `${MEDIA_BASE}/${cid}`;
          const isVideo = isVideoUrl(cid) || videoErrors.has(cid);

          if (isVideo) {
            return (
              <div
                key={cid}
                className={`relative ${
                  mediaCids.length === 1 ? 'aspect-video' : 'aspect-square'
                }`}
              >
                <video
                  src={url}
                  controls
                  className="w-full h-full object-cover"
                  onClick={(e) => e.stopPropagation()}
                  preload="metadata"
                >
                  <track kind="captions" />
                </video>
              </div>
            );
          }

          return (
            <button
              key={cid}
              type="button"
              className={`relative cursor-pointer ${
                mediaCids.length === 1 ? 'aspect-video' : 'aspect-square'
              } ${
                mediaCids.length === 3 && index === 0 ? 'row-span-2' : ''
              }`}
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                openLightbox(index);
              }}
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={url}
                alt={`Media ${index + 1}`}
                className="w-full h-full object-cover"
                onError={() => markAsVideo(cid)}
              />
            </button>
          );
        })}
      </div>

      {/* Lightbox */}
      {lightboxIndex !== null && (
        <div
          className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center"
          onClick={closeLightbox}
          onKeyDown={handleKeyDown}
          role="dialog"
          aria-modal="true"
          tabIndex={0}
        >
          {/* Close button */}
          <button
            onClick={closeLightbox}
            className="absolute top-4 right-4 text-white hover:text-gray-300 z-10 p-2"
            aria-label="Close"
          >
            <svg
              className="w-8 h-8"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>

          {/* Previous */}
          {lightboxIndex > 0 && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                setLightboxIndex(lightboxIndex - 1);
              }}
              className="absolute left-4 text-white hover:text-gray-300 p-2"
              aria-label="Previous"
            >
              <svg
                className="w-8 h-8"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M15 19l-7-7 7-7"
                />
              </svg>
            </button>
          )}

          {/* Image */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={`${MEDIA_BASE}/${mediaCids[lightboxIndex]}`}
            alt={`Media ${lightboxIndex + 1}`}
            className="max-w-[90vw] max-h-[90vh] object-contain"
            onClick={(e) => e.stopPropagation()}
          />

          {/* Next */}
          {lightboxIndex < mediaCids.length - 1 && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                setLightboxIndex(lightboxIndex + 1);
              }}
              className="absolute right-4 text-white hover:text-gray-300 p-2"
              aria-label="Next"
            >
              <svg
                className="w-8 h-8"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M9 5l7 7-7 7"
                />
              </svg>
            </button>
          )}

          {/* Counter */}
          {mediaCids.length > 1 && (
            <div className="absolute bottom-4 left-1/2 -translate-x-1/2 text-white text-sm bg-black/50 px-3 py-1 rounded-full">
              {lightboxIndex + 1} / {mediaCids.length}
            </div>
          )}
        </div>
      )}
    </>
  );
}
