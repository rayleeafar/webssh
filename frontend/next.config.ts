/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: 'standalone',
  /**
   * When NEXT_PUBLIC_API_URL is empty (same-origin deployment), proxy all
   * backend routes through Next.js so the frontend container can reach the
   * backend container without the browser needing a direct route to port 8080.
   *
   * BACKEND_URL defaults to http://backend:8080, matching the docker-compose
   * service name. Override it with the BACKEND_URL build/runtime env var.
   */
  async rewrites() {
    const backendUrl = process.env.BACKEND_URL || 'http://backend:8080'
    return [
      {
        source: '/api/:path*',
        destination: `${backendUrl}/api/:path*`,
      },
      {
        source: '/ws/:path*',
        destination: `${backendUrl}/ws/:path*`,
      },
      {
        source: '/health',
        destination: `${backendUrl}/health`,
      },
    ]
  },
}

export default nextConfig
