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

const description =
  "The planes outside a real window high over Toronto's harbour, live, drawn where you would see them.";

export const metadata: Metadata = {
  metadataBase: new URL("https://overhead-window.netlify.app"),
  title: "overhead",
  description,
  openGraph: { title: "overhead", description, type: "website", images: ["/og.jpg"] },
  twitter: { card: "summary_large_image" },
};

export const viewport: Viewport = { width: "device-width", initialScale: 1, themeColor: "#000000" };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={atkinson.className}>{children}</body>
    </html>
  );
}
