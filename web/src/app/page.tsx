import PostComposer from '@/components/PostComposer';
import Feed from '@/components/Feed';

export default function HomePage() {
  return (
    <div>
      {/* Header */}
      <header className="sticky top-0 z-10 bg-gray-950/80 backdrop-blur-md border-b border-gray-800 px-4 py-3">
        <h1 className="text-xl font-bold text-white">Home</h1>
      </header>

      {/* Composer */}
      <PostComposer />

      {/* Feed */}
      <Feed />
    </div>
  );
}
