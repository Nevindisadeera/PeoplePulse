import type { NextConfig } from "next";

// Address of the Go API. Browser requests to /api/v1 are forwarded there, so
// the app and the API share one origin and the refresh cookie stays first-party.
const API_URL = process.env.API_URL ?? "http://localhost:8080";

// Cache Components and Partial Prefetching are left off: every screen sits
// behind a client-side sign-in check and loads its data from the API in the
// browser, so there is no server-rendered content for them to cache or prefetch.
const nextConfig: NextConfig = {
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  async rewrites() {
    return [{ source: "/api/v1/:path*", destination: `${API_URL}/api/v1/:path*` }];
  },
};

export default nextConfig;
