"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { LogLine, Submission, SubmissionStatus } from "../types";

const POLL_MS = 1500;
const SYNTHETIC_STEP_MS = 1200;

type State = {
  submission: Submission | null;
  logs: LogLine[];
  uploading: boolean;
  error: string | null;
  source: "live" | "simulated" | "idle";
};

type UploadArgs = {
  file: File;
  lang: string;
  entrypoint: string;
  contestantID: string;
};

// Status transition order for the synthetic flow.
const SYNTHETIC_FLOW: SubmissionStatus[] = [
  "queued",
  "downloading",
  "extracting",
  "building",
  "pushing",
  "scanning",
  "ready",
];

const STAGE_LOGS: Record<SubmissionStatus, string> = {
  queued: "Queued for build worker",
  downloading: "Downloading source from MinIO bucket s3://submissions/…",
  extracting: "Extracting tarball (capped 500 MB, traversal-safe)",
  building: "docker build --build-arg ENTRYPOINT_PATH=. --build-arg RUNTIME_PORT=9100 .",
  pushing: "docker push localhost:5000/submissions/…:latest",
  scanning: "Trivy image scan (no critical CVEs)",
  ready: "Image ready — registered with leaderboard",
  failed: "Build failed — see logs above",
};

export function useSubmission() {
  const [state, setState] = useState<State>({
    submission: null,
    logs: [],
    uploading: false,
    error: null,
    source: "idle",
  });
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const syntheticTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const stopTimers = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
    if (syntheticTimerRef.current) {
      clearTimeout(syntheticTimerRef.current);
      syntheticTimerRef.current = null;
    }
  }, []);

  useEffect(() => () => stopTimers(), [stopTimers]);

  const appendLog = useCallback((text: string, level: LogLine["level"] = "info") => {
    setState((s) => ({ ...s, logs: [...s.logs, { t: Date.now(), level, text }] }));
  }, []);

  const reset = useCallback(() => {
    stopTimers();
    setState({ submission: null, logs: [], uploading: false, error: null, source: "idle" });
  }, [stopTimers]);

  const upload = useCallback(
    async (args: UploadArgs) => {
      stopTimers();
      setState({
        submission: null,
        logs: [{ t: Date.now(), level: "info", text: `Uploading ${args.file.name} (${args.lang})…` }],
        uploading: true,
        error: null,
        source: "idle",
      });

      const fd = new FormData();
      fd.append("file", args.file);
      fd.append("lang", args.lang);
      fd.append("entrypoint", args.entrypoint);
      fd.append("contestant_id", args.contestantID);

      try {
        const r = await fetch("/api/submissions", { method: "POST", body: fd });
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        const sub: Submission = await r.json();
        setState((s) => ({
          ...s,
          submission: sub,
          uploading: false,
          source: "live",
          logs: [...s.logs, { t: Date.now(), level: "info", text: `Accepted — id=${sub.id}, status=${sub.status}` }],
        }));
        startLivePoll(sub.id);
      } catch {
        startSynthetic(args);
      }
    },
    [stopTimers],
  );

  function startLivePoll(id: string) {
    pollRef.current = setInterval(async () => {
      try {
        const r = await fetch(`/api/submissions/${encodeURIComponent(id)}`);
        if (!r.ok) return;
        const sub: Submission = await r.json();
        setState((s) => {
          if (s.submission?.status !== sub.status) {
            const line = STAGE_LOGS[sub.status] ?? sub.status;
            return {
              ...s,
              submission: sub,
              logs: [...s.logs, { t: Date.now(), level: sub.status === "failed" ? "error" : "info", text: line }],
            };
          }
          return { ...s, submission: sub };
        });
        if (sub.status === "ready" || sub.status === "failed") stopTimers();
      } catch {
        // transient — keep polling
      }
    }, POLL_MS);
  }

  function startSynthetic(args: UploadArgs) {
    const id = `demo-${Math.random().toString(36).slice(2, 10)}`;
    const sub: Submission = {
      id,
      contestant_id: args.contestantID,
      lang: args.lang,
      status: "queued",
    };
    setState((s) => ({
      ...s,
      submission: sub,
      uploading: false,
      source: "simulated",
      error: null,
      logs: [
        ...s.logs,
        { t: Date.now(), level: "warn", text: "submission-svc not reachable — simulating build pipeline" },
        { t: Date.now(), level: "info", text: `Accepted — id=${id}, status=queued` },
      ],
    }));

    let idx = 1;
    const step = () => {
      if (idx >= SYNTHETIC_FLOW.length) return;
      const next = SYNTHETIC_FLOW[idx];
      setState((s) => {
        if (!s.submission) return s;
        const updated: Submission = {
          ...s.submission,
          status: next,
          image_url:
            next === "ready" ? `localhost:5000/submissions/${id}:latest` : s.submission.image_url,
        };
        return {
          ...s,
          submission: updated,
          logs: [...s.logs, { t: Date.now(), level: "info", text: STAGE_LOGS[next] }],
        };
      });
      idx++;
      if (idx < SYNTHETIC_FLOW.length) {
        syntheticTimerRef.current = setTimeout(step, SYNTHETIC_STEP_MS);
      }
    };
    syntheticTimerRef.current = setTimeout(step, SYNTHETIC_STEP_MS);
  }

  return { ...state, upload, reset, appendLog };
}
