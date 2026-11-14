import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Small self-contained server for the Docker image later on.
  output: "standalone",
  poweredByHeader: false,
  experimental: {
    // Stock CSVs may be up to 1 MB (the Stock Sync limit), plus multipart overhead.
    serverActions: { bodySizeLimit: "2mb" },
  },
};

export default nextConfig;
