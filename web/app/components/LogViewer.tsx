"use client";

import { useEffect, useRef } from "react";
import type { LogLine } from "../types";

export function LogViewer({ logs }: { logs: LogLine[] }) {
  const endRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [logs.length]);

  return (
    <div className="card log-viewer">
      <h3>Build log</h3>
      <div className="log-body">
        {logs.length === 0 && <div className="log-empty">No log lines yet.</div>}
        {logs.map((l, i) => (
          <div key={i} className={`log-line log-${l.level}`}>
            <span className="log-time">{new Date(l.t).toLocaleTimeString().split(" ")[0]}</span>
            <span className="log-text">{l.text}</span>
          </div>
        ))}
        <div ref={endRef} />
      </div>
    </div>
  );
}
