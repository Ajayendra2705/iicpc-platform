"use client";

import { useEffect, useRef, useState } from "react";
import type { ConnStatus, Entry, LivePayload } from "../types";

const POLL_INTERVAL_MS = 2000;
const WS_RECONNECT_MS = 3000;

type Args = {
  wsUrl: string;
  restUrl: string;
};

/**
 * Subscribes to the leaderboard via WebSocket and falls back to REST polling
 * when the socket cannot connect. Returns the latest entries and connection
 * status; consumers don't need to know which transport is active.
 */
export function useLeaderboard({ wsUrl, restUrl }: Args) {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [status, setStatus] = useState<ConnStatus>("connecting");
  const [lastUpdate, setLastUpdate] = useState<number | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    let cancelled = false;

    const stopPolling = () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };

    const fetchOnce = async () => {
      try {
        const r = await fetch(restUrl);
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const data: Entry[] = await r.json();
        if (cancelled) return;
        setEntries(data);
        setLastUpdate(Date.now());
      } catch {
        if (!cancelled) setStatus("error");
      }
    };

    const startPolling = () => {
      if (pollRef.current || cancelled) return;
      setStatus("polling");
      void fetchOnce();
      pollRef.current = setInterval(fetchOnce, POLL_INTERVAL_MS);
    };

    const openSocket = () => {
      if (cancelled) return;
      setStatus("connecting");
      let ws: WebSocket;
      try {
        ws = new WebSocket(wsUrl);
      } catch {
        startPolling();
        return;
      }
      wsRef.current = ws;

      ws.onopen = () => {
        if (cancelled) return;
        stopPolling();
        setStatus("connected");
      };

      ws.onmessage = (ev) => {
        if (cancelled) return;
        try {
          const payload: LivePayload = JSON.parse(ev.data);
          setEntries(payload.top ?? []);
          setLastUpdate(payload.at_unix_ms ?? Date.now());
        } catch {
          // ignore malformed frame
        }
      };

      ws.onerror = () => {
        // surface as polling — the server may be down; REST may still answer
        if (!cancelled) startPolling();
      };

      ws.onclose = () => {
        wsRef.current = null;
        if (cancelled) return;
        // try REST while we wait, then attempt to reconnect WS
        startPolling();
        reconnectRef.current = setTimeout(openSocket, WS_RECONNECT_MS);
      };
    };

    openSocket();

    return () => {
      cancelled = true;
      if (reconnectRef.current) clearTimeout(reconnectRef.current);
      stopPolling();
      if (wsRef.current) wsRef.current.close();
    };
  }, [wsUrl, restUrl]);

  return { entries, status, lastUpdate };
}
