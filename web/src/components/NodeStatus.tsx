'use client';

import { useNodeStatus } from '@/hooks/useNodeStatus';
import { useWebSocket } from '@/hooks/useWebSocket';

function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

export default function NodeStatus() {
  const { status } = useNodeStatus();
  const { isConnected } = useWebSocket();

  return (
    <div className="rounded-xl bg-gray-900 border border-gray-800 p-4">
      <h3 className="text-sm font-semibold text-white mb-3">Node Status</h3>
      <div className="space-y-2 text-sm">
        <div className="flex items-center justify-between">
          <span className="text-gray-400">Connection</span>
          <span className="flex items-center gap-1.5">
            <span
              className={`inline-block w-2 h-2 rounded-full ${
                isConnected ? 'bg-green-500' : 'bg-red-500'
              }`}
            />
            <span className="text-gray-300">
              {isConnected ? 'Online' : 'Offline'}
            </span>
          </span>
        </div>
        {status && (
          <>
            <div className="flex items-center justify-between">
              <span className="text-gray-400">Peers</span>
              <span className="text-gray-300">{status.peers}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-gray-400">Uptime</span>
              <span className="text-gray-300">
                {formatUptime(status.uptime)}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-gray-400">Bandwidth</span>
              <span className="text-gray-300">
                {formatBytes(status.bandwidth.totalIn)} /{' '}
                {formatBytes(status.bandwidth.totalOut)}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-gray-400">Storage</span>
              <span className="text-gray-300">
                {status.storage.usedGB.toFixed(1)} /{' '}
                {status.storage.maxGB.toFixed(0)} GB
              </span>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
