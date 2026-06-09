import type { ConnStatus } from "../types";

const labels: Record<ConnStatus, string> = {
  connecting: "Connecting…",
  connected: "Live (WebSocket)",
  polling: "Polling (REST fallback)",
  error: "Disconnected",
  demo: "Demo data (no backend)",
};

export function StatusBadge({ status }: { status: ConnStatus }) {
  return (
    <span className={`badge ${status}`}>
      <span className="dot" />
      {labels[status]}
    </span>
  );
}
