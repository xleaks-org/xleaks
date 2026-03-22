import type { Metadata } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import './globals.css';
import Sidebar from '@/components/Sidebar';
import NodeStatus from '@/components/NodeStatus';
import TrendingList from '@/components/TrendingList';
import SearchBar from '@/components/SearchBar';

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
});

export const metadata: Metadata = {
  title: 'XLeaks',
  description: 'Decentralized P2P social platform - censorship-resistant',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased dark`}
    >
      <body className="min-h-full bg-gray-950 text-white">
        <div className="mx-auto flex max-w-7xl">
          {/* Left sidebar - Navigation */}
          <div className="hidden md:block md:w-64 shrink-0">
            <div className="sticky top-0 h-screen">
              <Sidebar />
            </div>
          </div>

          {/* Mobile sidebar (rendered outside normal flow) */}
          <div className="md:hidden">
            <Sidebar />
          </div>

          {/* Center content */}
          <main className="flex-1 min-w-0 border-x border-gray-800 min-h-screen">
            {children}
          </main>

          {/* Right sidebar - Status/Trending */}
          <div className="hidden lg:block w-80 shrink-0">
            <div className="sticky top-0 p-4 space-y-4">
              <SearchBar />

              <NodeStatus />

              <TrendingList />

              {/* About */}
              <div className="rounded-xl bg-gray-900 border border-gray-800 p-4">
                <h3 className="text-sm font-semibold text-white mb-2">
                  About XLeaks
                </h3>
                <p className="text-xs text-gray-400">
                  Decentralized peer-to-peer social platform. Your data, your
                  rules. No central authority, no censorship.
                </p>
              </div>
            </div>
          </div>
        </div>
      </body>
    </html>
  );
}
