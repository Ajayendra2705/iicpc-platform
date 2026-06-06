/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Emit a self-contained server bundle for the Docker image. Includes only
  // the runtime files Next.js actually needs — no node_modules copy.
  output: 'standalone',
  // Proxy REST calls to leaderboard-svc to avoid browser CORS in dev.
  async rewrites() {
    // Use 127.0.0.1 (not localhost) — on Windows + Node 22 localhost resolves
    // to ::1 first, but Go listeners default to IPv4-only, causing ECONNREFUSED.
    const leaderboard = process.env.LEADERBOARD_URL || 'http://127.0.0.1:8086';
    const aggregator = process.env.AGGREGATOR_URL || 'http://127.0.0.1:8084';
    const validator = process.env.VALIDATOR_URL || 'http://127.0.0.1:8085';
    const submission = process.env.SUBMISSION_URL || 'http://127.0.0.1:8080';
    return [
      { source: '/api/leaderboard', destination: `${leaderboard}/leaderboard` },
      { source: '/api/metrics/:id', destination: `${aggregator}/metrics/:id` },
      { source: '/api/validate/:id', destination: `${validator}/validate/:id` },
      { source: '/api/submissions', destination: `${submission}/submissions` },
      { source: '/api/submissions/:id', destination: `${submission}/submissions/:id` },
      { source: '/api/submissions/:id/logs', destination: `${submission}/submissions/:id/logs` },
    ];
  },
};

module.exports = nextConfig;
