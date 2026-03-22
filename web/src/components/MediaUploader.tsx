'use client';

import { useState, useCallback, useRef } from 'react';
import { uploadMedia } from '@/lib/api';

const ACCEPTED_TYPES = [
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/gif',
  'video/mp4',
  'video/webm',
];
const MAX_FILE_SIZE = 100 * 1024 * 1024; // 100MB
const DEFAULT_MAX_FILES = 10;

interface FilePreview {
  file: File;
  preview: string;
  isVideo: boolean;
}

export default function MediaUploader({
  onUpload,
  maxFiles = DEFAULT_MAX_FILES,
}: {
  onUpload: (cids: string[]) => void;
  maxFiles?: number;
}) {
  const [files, setFiles] = useState<FilePreview[]>([]);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState('');
  const [dragging, setDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const validateFile = useCallback(
    (file: File): string | null => {
      if (!ACCEPTED_TYPES.includes(file.type)) {
        return `"${file.name}" has an unsupported file type. Accepted: JPEG, PNG, WebP, GIF, MP4, WebM.`;
      }
      if (file.size > MAX_FILE_SIZE) {
        return `"${file.name}" exceeds the 100MB size limit.`;
      }
      return null;
    },
    []
  );

  const addFiles = useCallback(
    (newFiles: FileList | File[]) => {
      setError('');
      const fileArray = Array.from(newFiles);

      for (const file of fileArray) {
        const validationError = validateFile(file);
        if (validationError) {
          setError(validationError);
          return;
        }
      }

      setFiles((prev) => {
        const combined = [...prev];
        for (const file of fileArray) {
          if (combined.length >= maxFiles) {
            setError(`Maximum ${maxFiles} files allowed.`);
            break;
          }
          const isVideo = file.type.startsWith('video/');
          const preview = URL.createObjectURL(file);
          combined.push({ file, preview, isVideo });
        }
        return combined;
      });
    },
    [maxFiles, validateFile]
  );

  const removeFile = useCallback((index: number) => {
    setFiles((prev) => {
      const updated = [...prev];
      URL.revokeObjectURL(updated[index].preview);
      updated.splice(index, 1);
      return updated;
    });
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragging(false);
      if (e.dataTransfer.files.length > 0) {
        addFiles(e.dataTransfer.files);
      }
    },
    [addFiles]
  );

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files && e.target.files.length > 0) {
        addFiles(e.target.files);
      }
      if (fileInputRef.current) fileInputRef.current.value = '';
    },
    [addFiles]
  );

  const handleUpload = useCallback(async () => {
    if (files.length === 0) return;
    setUploading(true);
    setError('');
    setProgress(0);

    const cids: string[] = [];
    try {
      for (let i = 0; i < files.length; i++) {
        const result = await uploadMedia(files[i].file);
        cids.push(result.cid);
        setProgress(Math.round(((i + 1) / files.length) * 100));
      }
      // Clean up previews
      files.forEach((f) => URL.revokeObjectURL(f.preview));
      setFiles([]);
      onUpload(cids);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Upload failed. Please try again.'
      );
    } finally {
      setUploading(false);
    }
  }, [files, onUpload]);

  return (
    <div className="space-y-3">
      {/* Drop zone */}
      <div
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
        className={`border-2 border-dashed rounded-xl p-6 text-center cursor-pointer transition-colors ${
          dragging
            ? 'border-blue-500 bg-blue-500/10'
            : 'border-gray-700 hover:border-gray-600 bg-gray-900/50'
        }`}
      >
        <input
          ref={fileInputRef}
          type="file"
          accept={ACCEPTED_TYPES.join(',')}
          multiple
          className="hidden"
          onChange={handleFileSelect}
        />
        <svg
          className="w-8 h-8 mx-auto mb-2 text-gray-500"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={1.5}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 16.5V9.75m0 0l3 3m-3-3l-3 3M6.75 19.5a4.5 4.5 0 01-1.41-8.775 5.25 5.25 0 0110.338-2.338 5.25 5.25 0 013.987 6.238A4.5 4.5 0 0118 19.5H6.75z"
          />
        </svg>
        <p className="text-sm text-gray-400">
          Drop files here or click to browse
        </p>
        <p className="text-xs text-gray-600 mt-1">
          Images (JPEG, PNG, WebP, GIF) and videos (MP4, WebM) -- max 100MB each
        </p>
      </div>

      {/* Error */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 text-sm text-red-400">
          {error}
        </div>
      )}

      {/* Previews */}
      {files.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {files.map((f, index) => (
            <div
              key={f.preview}
              className="relative w-20 h-20 rounded-lg overflow-hidden bg-gray-800 border border-gray-700"
            >
              {f.isVideo ? (
                <video
                  src={f.preview}
                  className="w-full h-full object-cover"
                  muted
                >
                  <track kind="captions" />
                </video>
              ) : (
                /* eslint-disable-next-line @next/next/no-img-element */
                <img
                  src={f.preview}
                  alt={`Preview ${index + 1}`}
                  className="w-full h-full object-cover"
                />
              )}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  removeFile(index);
                }}
                className="absolute top-0.5 right-0.5 w-5 h-5 bg-black/70 rounded-full flex items-center justify-center text-white text-xs hover:bg-black"
                aria-label="Remove file"
              >
                x
              </button>
              {f.isVideo && (
                <div className="absolute bottom-0.5 left-0.5 bg-black/70 rounded px-1 text-[10px] text-white">
                  Video
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Upload button + progress */}
      {files.length > 0 && (
        <div className="flex items-center gap-3">
          <button
            onClick={handleUpload}
            disabled={uploading}
            className="bg-blue-500 hover:bg-blue-600 disabled:opacity-50 text-white font-bold px-5 py-2 rounded-full transition-colors text-sm"
          >
            {uploading ? `Uploading... ${progress}%` : `Upload ${files.length} file${files.length > 1 ? 's' : ''}`}
          </button>
          {uploading && (
            <div className="flex-1 bg-gray-800 rounded-full h-2 overflow-hidden">
              <div
                className="bg-blue-500 h-full transition-all duration-300"
                style={{ width: `${progress}%` }}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
