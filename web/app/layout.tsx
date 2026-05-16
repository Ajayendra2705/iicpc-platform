import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "IICPC Leaderboard",
  description: "Live trading-infrastructure benchmark leaderboard.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
