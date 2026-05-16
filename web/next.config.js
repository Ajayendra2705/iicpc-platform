/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Proxy REST calls to leaderboard-svc to avoid browser CORS in dev.
  async rewrites() {
    const leaderboard = process.env.LEADERBOARD_URL || 'http://localhost:8086';
    return [
      {
        source: '/api/leaderboard',
        destination: `${leaderboard}/leaderboard`,
      },
    ];
  },
};

module.exports = nextConfig;
