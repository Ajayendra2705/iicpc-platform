export type Entry = {
  contestant_id: string;
  score: number;
};

export type LivePayload = {
  top: Entry[];
  at_unix_ms: number;
};

export type ConnStatus = "connecting" | "connected" | "polling" | "error";

export type SortKey = "rank" | "contestant_id" | "score";
export type SortDir = "asc" | "desc";

export type MetricsSnapshot = {
  contestant_id: string;
  count: number;
  rejected: number;
  timeouts: number;
  tps: number;
  p50_ns: number;
  p90_ns: number;
  p99_ns: number;
  p999_ns: number;
};

export type ValidatorReport = {
  contestant_id: string;
  total_checked: number;
  mismatches: number;
  correctness: number;
};

export type TPSSample = {
  t: number; // unix ms
  tps: number;
};

export type SubmissionStatus =
  | "queued"
  | "downloading"
  | "extracting"
  | "building"
  | "pushing"
  | "scanning"
  | "ready"
  | "failed";

export type Submission = {
  id: string;
  contestant_id: string;
  lang: string;
  status: SubmissionStatus;
  image_url?: string;
  error?: string;
};

export type LogLine = {
  t: number; // unix ms
  level: "info" | "warn" | "error";
  text: string;
};
