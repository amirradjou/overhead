"use client";

import { useEffect, useState } from "react";
import { frameAt, type Recording } from "./replay";
import type { View } from "./types";

export type Status = "connecting" | "live" | "reconnecting" | "replay";

// The server sends a snapshot every 2 s. A stream silent for this long is treated as dead and reopened: browsers
// retry dropped streams by themselves, but not one that ended with an HTTP error, and a TV left on all day must
// never sit on a frozen picture.
const silence = 15_000;

// A first-time visitor waits no longer than this for a stream that neither answers nor fails, as one through a tunnel
// may while the machine behind it sleeps, before the page plays its recording.
const patience = 5_000;

/**
 * Where to read the API from. A build that names its API keeps to it: the public page must not show whatever a crafted
 * `?api=` link points it at, under its own address.
 */
export function apiBase(built: string | undefined, search: string): string {
  return built || (new URLSearchParams(search).get("api") ?? "");
}

/**
 * The live picture from `overhead live`. The API is on the page's own origin unless the build says otherwise
 * (NEXT_PUBLIC_OVERHEAD_API, for a page hosted apart from it) or the address bar does (`?api=http://host:8090`, handy
 * with `pnpm dev`; see apiBase).
 *
 * A page built with NEXT_PUBLIC_OVERHEAD_REPLAY=1 plays its recording while the feed is away, and returns to live the
 * moment the feed does; the screen says which it is showing. `?replay` plays the recording on demand. A TV's page
 * never falls back: at home, planes that are not there would be a lie.
 */
export function useLiveView(): { view: View | null; status: Status } {
  const [view, setView] = useState<View | null>(null);
  const [status, setStatus] = useState<Status>("connecting");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const base = apiBase(process.env.NEXT_PUBLIC_OVERHEAD_API, window.location.search);
    const onDemand = params.has("replay");
    const canReplay = onDemand || process.env.NEXT_PUBLIC_OVERHEAD_REPLAY === "1";
    let source: EventSource | null = null;
    let lastMessage = Date.now();
    let everLive = false;
    let wantReplay = false;
    let recording: Recording | null = null;
    let player: number | undefined;

    const stopReplay = () => {
      wantReplay = false;
      window.clearInterval(player);
      player = undefined;
    };

    const startReplay = async () => {
      if (wantReplay || !canReplay) return;
      wantReplay = true;
      try {
        recording ??= (await (await fetch("/replay.json")).json()) as Recording;
      } catch {
        wantReplay = false; // tried again the next time the feed is found silent
        return;
      }
      if (!wantReplay) return; // the feed came back in the meantime
      const rec = recording;
      const started = Date.now();
      const show = () => {
        setView(frameAt(rec, Date.now() - started, new Date()));
        setStatus("replay");
      };
      show();
      player = window.setInterval(show, rec.step_s * 1000);
    };

    const open = () => {
      source?.close();
      source = new EventSource(`${base}/api/events`);
      source.addEventListener("snapshot", (e) => {
        lastMessage = Date.now();
        everLive = true;
        stopReplay();
        try {
          setView(JSON.parse((e as MessageEvent<string>).data) as View);
          setStatus("live");
        } catch {
          // A garbled message is skipped; the next one replaces the whole picture anyway.
        }
      });
      source.onerror = () => {
        setStatus((s) => (s === "replay" ? s : "reconnecting"));
        // A page just opened has nothing to show: a stream that fails at once needs no long wait to be sure of it.
        if (!everLive) void startReplay();
      };
    };

    if (onDemand) {
      void startReplay();
      return stopReplay;
    }

    open();
    const firstLook = window.setTimeout(() => {
      if (!everLive) void startReplay();
    }, patience);
    const watchdog = window.setInterval(() => {
      if (Date.now() - lastMessage > silence) {
        setStatus((s) => (s === "replay" ? s : "reconnecting"));
        void startReplay();
        lastMessage = Date.now(); // wait another full period before trying again
        open();
      }
    }, 5_000);
    return () => {
      window.clearTimeout(firstLook);
      window.clearInterval(watchdog);
      stopReplay();
      source?.close();
    };
  }, []);

  return { view, status };
}
