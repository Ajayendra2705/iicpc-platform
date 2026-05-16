"use client";

import type { Submission, SubmissionStatus as Status } from "../types";

const FLOW: Status[] = ["queued", "downloading", "extracting", "building", "pushing", "scanning", "ready"];

const labels: Record<Status, string> = {
  queued: "Queued",
  downloading: "Downloading",
  extracting: "Extracting",
  building: "Building",
  pushing: "Pushing",
  scanning: "Scanning",
  ready: "Ready",
  failed: "Failed",
};

export function SubmissionStatus({ submission }: { submission: Submission }) {
  const currentIdx = FLOW.indexOf(submission.status);
  const failed = submission.status === "failed";

  return (
    <div className="card submission-status">
      <div className="status-head">
        <div>
          <div className="stat-label">Submission ID</div>
          <div className="status-id">{submission.id}</div>
        </div>
        <div>
          <div className="stat-label">Status</div>
          <div className={`status-pill ${submission.status}`}>{labels[submission.status]}</div>
        </div>
      </div>

      <div className="status-flow">
        {FLOW.map((s, i) => {
          const reached = !failed && i <= currentIdx;
          const current = !failed && i === currentIdx && submission.status !== "ready";
          return (
            <div key={s} className={`step ${reached ? "reached" : ""} ${current ? "current" : ""}`}>
              <div className="step-dot" />
              <div className="step-label">{labels[s]}</div>
            </div>
          );
        })}
      </div>

      {submission.image_url && (
        <div className="image-url">
          <span className="stat-label">Image</span>
          <code>{submission.image_url}</code>
        </div>
      )}

      {submission.error && (
        <div className="error-text">Error: {submission.error}</div>
      )}
    </div>
  );
}
