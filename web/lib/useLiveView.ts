"use client";

import { useEffect, useState } from "react";
import type { View } from "./types";

export type Status = "connecting" | "live" | "reconnecting";

// The server sends a snapshot every 2 s. A stream silent for this long is treated as dead and reopened: browsers
// retry dropped streams by themselves, but not one that ended with an HTTP error, and a TV left on all day must
// never sit on a frozen picture.
const silence = 15_000;

/**
 * The live picture from `overhead live`. The API is on the page's own origin unless the address bar says
 * `?api=http://host:8090` (handy with `pnpm dev`).
 */
export function useLiveView(): { view: View | null; status: Status } {
  const [view, setView] = useState<View | null>(null);
  const [status, setStatus] = useState<Status>("connecting");

  useEffect(() => {
    const base = new URLSearchParams(window.location.search).get("api") ?? "";
    let source: EventSource | null = null;
    let lastMessage = Date.now();

    const open = () => {
      source?.close();
      source = new EventSource(`${base}/api/events`);
      source.addEventListener("snapshot", (e) => {
        lastMessage = Date.now();
        try {
          setView(JSON.parse((e as MessageEvent<string>).data) as View);
          setStatus("live");
        } catch {
          // A garbled message is skipped; the next one replaces the whole picture anyway.
        }
      });
      source.onerror = () => setStatus("reconnecting");
    };

    open();
    const watchdog = window.setInterval(() => {
      if (Date.now() - lastMessage > silence) {
        setStatus("reconnecting");
        lastMessage = Date.now(); // wait another full period before trying again
        open();
      }
    }, 5_000);
    return () => {
      window.clearInterval(watchdog);
      source?.close();
    };
  }, []);

  return { view, status };
}
