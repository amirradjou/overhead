import type { Metadata, Viewport } from "next";
import localFont from "next/font/local";
import "./globals.css";

// Atkinson Hyperlegible Next was drawn for legibility at a glance and at a distance: what a screen read from the
// sofa needs. Bundled from the npm package (OFL), so neither the build nor the TV fetches anything from a font host.
const atkinson = localFont({
  src: "../node_modules/@fontsource-variable/atkinson-hyperlegible-next/files/atkinson-hyperlegible-next-latin-wght-normal.woff2",
  weight: "200 800",
  display: "swap",
});

export const metadata: Metadata = {
  title: "overhead",
  description: "The planes you can see from your window, live.",
};

export const viewport: Viewport = { width: "device-width", initialScale: 1, themeColor: "#000000" };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={atkinson.className}>{children}</body>
    </html>
  );
}
