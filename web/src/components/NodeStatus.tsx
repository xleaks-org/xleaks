'use client';

import { useNodeStatus } from '@/hooks/useNodeStatus';
import { useWebSocket } from '@/hooks/useWebSocket';
import { formatBytes, formatDuration } from '@/lib/formatters';


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
                {formatDuration(status.uptime)}
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
