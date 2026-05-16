/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Proxy REST calls to leaderboard-svc to avoid browser CORS in dev.
  async rewrites() {
    // Use 127.0.0.1 (not localhost) — on Windows + Node 22 localhost resolves
    // to ::1 first, but Go listeners default to IPv4-only, causing ECONNREFUSED.
    const leaderboard = process.env.LEADERBOARD_URL || 'http://127.0.0.1:8086';
    return [
      {
        source: '/api/leaderboard',
        destination: `${leaderboard}/leaderboard`,
      },
    ];
  },
};

module.exports = nextConfig;
