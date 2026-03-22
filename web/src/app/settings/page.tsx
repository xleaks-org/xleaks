'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useNodeStatus } from '@/hooks/useNodeStatus';
import { getActiveIdentity, getNodeConfig, updateNodeConfig } from '@/lib/api';
import { formatBytes, formatDuration } from '@/lib/formatters';

type FontSize = 'small' | 'medium' | 'large';
const FONT_SIZE_MAP: Record<FontSize, string> = {
  small: '14px',
  medium: '16px',
  large: '18px',
};

export default function SettingsPage() {
  const router = useRouter();
  const { status } = useNodeStatus();

  // Identity state
  const [identityAddress, setIdentityAddress] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  // Node config state
  const [maxStorageGB, setMaxStorageGB] = useState<number>(10);
  const [savingStorage, setSavingStorage] = useState(false);
  const [bootstrapPeers, setBootstrapPeers] = useState<string[]>([]);
  const [relayEnabled, setRelayEnabled] = useState(false);

  // Display state
  const [darkMode, setDarkMode] = useState(true);
  const [fontSize, setFontSize] = useState<FontSize>('medium');

  // Load identity
  useEffect(() => {
    getActiveIdentity().then((identity) => {
      if (identity?.address) {
        setIdentityAddress(identity.address);
      }
    });
  }, []);

  // Load node config
  useEffect(() => {
    getNodeConfig()
      .then((config) => {
        setMaxStorageGB(config.maxStorageGB);
        setBootstrapPeers(config.bootstrapPeers);
        setRelayEnabled(config.relayEnabled);
      })
      .catch(() => {
        // Config endpoint not available
      });
  }, []);

  // Load display preferences from localStorage
  useEffect(() => {
    const savedTheme = localStorage.getItem('xleaks-theme');
    if (savedTheme === 'light') {
      setDarkMode(false);
      document.documentElement.classList.add('light');
    }

    const savedFontSize = localStorage.getItem('xleaks-font-size') as FontSize | null;
    if (savedFontSize && FONT_SIZE_MAP[savedFontSize]) {
      setFontSize(savedFontSize);
      document.documentElement.style.setProperty('--app-font-size', FONT_SIZE_MAP[savedFontSize]);
    }
  }, []);

  const handleCopyAddress = useCallback(() => {
    if (identityAddress) {
      navigator.clipboard?.writeText(identityAddress);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [identityAddress]);

  const handleToggleDarkMode = useCallback(() => {
    setDarkMode((prev) => {
      const next = !prev;
      if (next) {
        document.documentElement.classList.remove('light');
        localStorage.setItem('xleaks-theme', 'dark');
      } else {
        document.documentElement.classList.add('light');
        localStorage.setItem('xleaks-theme', 'light');
      }
      return next;
    });
  }, []);

  const handleFontSizeChange = useCallback((size: FontSize) => {
    setFontSize(size);
    localStorage.setItem('xleaks-font-size', size);
    document.documentElement.style.setProperty('--app-font-size', FONT_SIZE_MAP[size]);
  }, []);

  const handleSaveMaxStorage = useCallback(async () => {
    setSavingStorage(true);
    try {
      await updateNodeConfig({ maxStorageGB });
    } catch {
      // Silently handle
    } finally {
      setSavingStorage(false);
    }
  }, [maxStorageGB]);

  const storagePercent =
    status && status.storage.maxGB > 0
      ? Math.min(100, (status.storage.usedGB / status.storage.maxGB) * 100)
      : 0;

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
            {/* Active identity address */}
            <div>
              <label className="block text-sm text-gray-400 mb-1">
                Active Identity
              </label>
              <div className="flex items-center gap-2">
                <code className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-gray-300 overflow-x-auto">
                  {identityAddress || 'Not connected'}
                </code>
                <button
                  onClick={handleCopyAddress}
                  disabled={!identityAddress}
                  className="text-blue-500 hover:text-blue-400 text-sm shrink-0 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  {copied ? 'Copied!' : 'Copy'}
                </button>
              </div>
            </div>

            {/* Backup seed phrase */}
            <div>
              <button
                disabled
                className="bg-gray-800 text-gray-500 text-sm px-4 py-2 rounded-lg cursor-not-allowed"
              >
                Backup Seed Phrase
              </button>
              <p className="text-xs text-gray-500 mt-1">
                Requires re-unlock to view seed phrase
              </p>
            </div>

            {/* Action buttons */}
            <div className="flex flex-wrap gap-3">
              <a
                href="/api/identity/export"
                className="bg-gray-800 hover:bg-gray-700 text-white text-sm px-4 py-2 rounded-lg transition-colors inline-block"
              >
                Export Key
              </a>
              <button
                onClick={() => router.push('/onboarding')}
                className="bg-gray-800 hover:bg-gray-700 text-white text-sm px-4 py-2 rounded-lg transition-colors"
              >
                Create New Identity
              </button>
              <button
                onClick={() => router.push('/onboarding')}
                className="bg-gray-800 hover:bg-gray-700 text-white text-sm px-4 py-2 rounded-lg transition-colors"
              >
                Import Identity
              </button>
            </div>
          </div>
        </section>

        {/* Node Section */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-white mb-4">Node</h2>
          <div className="space-y-4">
            {/* Storage usage bar */}
            <div>
              <div className="flex items-center justify-between text-sm mb-1">
                <span className="text-gray-400">Storage Usage</span>
                <span className="text-white">
                  {status
                    ? `${status.storage.usedGB.toFixed(1)} / ${status.storage.maxGB.toFixed(0)} GB`
                    : 'N/A'}
                </span>
              </div>
              <div className="w-full h-2 bg-gray-800 rounded-full overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all ${
                    storagePercent > 90
                      ? 'bg-red-500'
                      : storagePercent > 70
                        ? 'bg-yellow-500'
                        : 'bg-blue-500'
                  }`}
                  style={{ width: `${storagePercent}%` }}
                />
              </div>
            </div>

            {/* Max storage input */}
            <div>
              <label className="block text-sm text-gray-400 mb-1">
                Max Storage (GB)
              </label>
              <div className="flex items-center gap-2">
                <input
                  type="number"
                  min={1}
                  max={1000}
                  value={maxStorageGB}
                  onChange={(e) => setMaxStorageGB(Number(e.target.value))}
                  className="w-24 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
                />
                <button
                  onClick={handleSaveMaxStorage}
                  disabled={savingStorage}
                  className="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-sm px-4 py-2 rounded-lg transition-colors"
                >
                  {savingStorage ? 'Saving...' : 'Save'}
                </button>
              </div>
            </div>

            {/* Node stats */}
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-sm text-gray-400">Connected Peers</span>
                <span className="text-sm text-white">
                  {status?.peers ?? 'N/A'}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-gray-400">Bandwidth In</span>
                <span className="text-sm text-white">
                  {status ? formatBytes(status.bandwidth.totalIn) : 'N/A'}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-gray-400">Bandwidth Out</span>
                <span className="text-sm text-white">
                  {status ? formatBytes(status.bandwidth.totalOut) : 'N/A'}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-gray-400">Uptime</span>
                <span className="text-sm text-white">
                  {status ? formatDuration(status.uptime) : 'N/A'}
                </span>
              </div>
            </div>
          </div>
        </section>

        {/* Display Section */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-white mb-4">Display</h2>
          <div className="space-y-4">
            {/* Dark/light mode toggle */}
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-white">Dark Mode</p>
                <p className="text-xs text-gray-400">
                  Toggle between dark and light theme
                </p>
              </div>
              <button
                onClick={handleToggleDarkMode}
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

            {/* Font size selector */}
            <div>
              <p className="text-sm text-white mb-2">Font Size</p>
              <div className="flex gap-2">
                {(['small', 'medium', 'large'] as FontSize[]).map((size) => (
                  <button
                    key={size}
                    onClick={() => handleFontSizeChange(size)}
                    className={`px-4 py-2 rounded-lg text-sm capitalize transition-colors ${
                      fontSize === size
                        ? 'bg-blue-600 text-white'
                        : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
                    }`}
                  >
                    {size} ({FONT_SIZE_MAP[size]})
                  </button>
                ))}
              </div>
            </div>
          </div>
        </section>

        {/* Network Section */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-white mb-4">Network</h2>
          <div className="space-y-4">
            {/* Relay enabled */}
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Relay Enabled</span>
              <span
                className={`text-sm font-medium ${
                  relayEnabled ? 'text-green-400' : 'text-gray-500'
                }`}
              >
                {relayEnabled ? 'Yes' : 'No'}
              </span>
            </div>

            {/* Bootstrap peers */}
            <div>
              <p className="text-sm text-gray-400 mb-2">Bootstrap Peers</p>
              {bootstrapPeers.length > 0 ? (
                <ul className="space-y-1">
                  {bootstrapPeers.map((peer, i) => (
                    <li
                      key={i}
                      className="text-xs text-gray-300 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 font-mono break-all"
                    >
                      {peer}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-sm text-gray-500">No bootstrap peers configured</p>
              )}
            </div>
          </div>
        </section>

        {/* About Section */}
        <section className="p-4">
          <h2 className="text-lg font-bold text-white mb-4">About</h2>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Version</span>
              <span className="text-sm text-white">1.0.0</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">Protocol</span>
              <span className="text-sm text-white">XLeaks Protocol v1</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">GitHub</span>
              <a
                href="https://github.com/xleaks-org/xleaks"
                target="_blank"
                rel="noopener noreferrer"
                className="text-sm text-blue-400 hover:text-blue-300 transition-colors"
              >
                xleaks-org/xleaks
              </a>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-400">License</span>
              <span className="text-sm text-white">AGPL-3.0</span>
            </div>
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
