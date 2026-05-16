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
