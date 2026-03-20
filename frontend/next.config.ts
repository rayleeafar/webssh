/** @type {import('next').NextConfig} */
const isBuild = process.env.NEXT_STATIC_EXPORT === 'true'

const nextConfig = isBuild
  ? {
      // Static export for single-binary embedding in Go
      output: 'export',
      trailingSlash: true,
      env: {
        NEXT_PUBLIC_WS_URL: process.env.NEXT_PUBLIC_WS_URL || '',
      },
    }
  : {
      // Default: standalone mode for Docker / development
      reactStrictMode: true,
      output: 'standalone',
      env: {
        // Expose NEXT_PUBLIC_WS_URL so it is available in the browser bundle even
        // when set only as a Docker build-arg (Next.js requires NEXT_PUBLIC_ prefix
        // for client-side access; listing it here ensures it survives the build).
        NEXT_PUBLIC_WS_URL: process.env.NEXT_PUBLIC_WS_URL || '',
      },
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
