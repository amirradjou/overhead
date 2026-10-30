import type { NextConfig } from "next";

// A static export: `overhead live -web web/out` serves it from the same origin as the API, so the TV needs no
// Node server and no CORS.
const config: NextConfig = {
  output: "export",
  reactStrictMode: true,
  images: { unoptimized: true },
};

export default config;
