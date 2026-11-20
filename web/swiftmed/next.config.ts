import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Small self-contained server for the Docker image later on.
  output: "standalone",
  poweredByHeader: false,
};

export default nextConfig;
