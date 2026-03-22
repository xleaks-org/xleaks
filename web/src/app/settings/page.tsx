'use client';

import { useState } from 'react';
import { useNodeStatus } from '@/hooks/useNodeStatus';

export default function SettingsPage() {
  const { status } = useNodeStatus();
  const [darkMode, setDarkMode] = useState(true);
  const [showSeed, setShowSeed] = useState(false);

  return (
    <div>
      {/* Header */}
      <header className="sticky top-0 z-10 bg-gray-950/80 backdrop-blur-md border-b border-gray-800 px-4 py-3">
        <h1 className="text-xl font-bold text-white">Settings</h1>
      </header>

      <div className="divide-y divide-gray-800">
        {/* Identity Section */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-white mb-4">Identity</h2>
          <div className="space-y-4">
            <div>
              <label className="block text-sm text-gray-400 mb-1">
                Public Key
              </label>
              <div className="flex items-center gap-2">
                <code className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-gray-300 overflow-x-auto">
                  Not connected
                </code>
                <button className="text-blue-500 hover:text-blue-400 text-sm shrink-0">
                  Copy
                </button>
              </div>
            </div>

            <div>
              <button
                onClick={() => setShowSeed(!showSeed)}
                className="text-sm text-blue-500 hover:text-blue-400 transition-colors"
              >
                {showSeed ? 'Hide' : 'Show'} Seed Phrase
              </button>
              {showSeed && (
                <div className="mt-2 bg-red-500/10 border border-red-500/30 rounded-lg p-3">
                  <p className="text-xs text-red-400 mb-2">
                    Warning: Never share your seed phrase with anyone. Anyone
                    with this phrase can control your identity.
                  </p>
                  <code className="block text-sm text-gray-300 bg-gray-800 rounded p-2">
                    Connect to a node to view seed phrase
                  </code>
                </div>
              )}
            </div>

            <button className="bg-gray-800 hover:bg-gray-700 text-white text-sm px-4 py-2 rounded-lg transition-colors">
              Export Identity
            </button>
          </div>
        </section>

        {/* Node Section */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-white mb-4">Node</h2>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Connected Peers</span>
              <span className="text-sm text-white">
                {status?.peers ?? 'N/A'}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Storage Used</span>
              <span className="text-sm text-white">
                {status
                  ? `${status.storage.usedGB.toFixed(1)} / ${status.storage.maxGB.toFixed(0)} GB`
                  : 'N/A'}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Uptime</span>
              <span className="text-sm text-white">
                {status
                  ? `${Math.floor(status.uptime / 3600)}h ${Math.floor((status.uptime % 3600) / 60)}m`
                  : 'N/A'}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Bandwidth In</span>
              <span className="text-sm text-white">
                {status
                  ? `${(status.bandwidth.totalIn / (1024 * 1024)).toFixed(1)} MB`
                  : 'N/A'}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Bandwidth Out</span>
              <span className="text-sm text-white">
                {status
                  ? `${(status.bandwidth.totalOut / (1024 * 1024)).toFixed(1)} MB`
                  : 'N/A'}
              </span>
            </div>
          </div>
        </section>

        {/* Display Section */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-white mb-4">Display</h2>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-white">Dark Mode</p>
              <p className="text-xs text-gray-400">
                Toggle between dark and light theme
              </p>
            </div>
            <button
              onClick={() => setDarkMode(!darkMode)}
              className={`relative w-11 h-6 rounded-full transition-colors ${
                darkMode ? 'bg-blue-500' : 'bg-gray-600'
              }`}
              role="switch"
              aria-checked={darkMode}
            >
              <span
                className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full transition-transform ${
                  darkMode ? 'translate-x-5' : 'translate-x-0'
                }`}
              />
            </button>
          </div>
        </section>

        {/* Danger zone */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-red-400 mb-4">Danger Zone</h2>
          <button className="bg-red-500/10 hover:bg-red-500/20 text-red-400 text-sm px-4 py-2 rounded-lg border border-red-500/30 transition-colors">
            Delete Local Data
          </button>
        </section>
      </div>
    </div>
  );
}
