"use client";

import Link from "next/link";
import { LogViewer } from "../components/LogViewer";
import { SubmissionStatus } from "../components/SubmissionStatus";
import { UploadForm } from "../components/UploadForm";
import { useSubmission } from "../hooks/useSubmission";

export default function SubmitPage() {
  const { submission, logs, uploading, source, upload, reset } = useSubmission();

  return (
    <main>
      <div className="header">
        <div>
          <Link href="/" className="back-link">← Back to leaderboard</Link>
          <h1>Submit a contestant</h1>
          <div className="subtitle">
            Upload a source archive. The platform builds it under gVisor, runs a
            sandboxed pod, and feeds bot traffic against it.
            {source === "simulated" && (
              <>
                {" · "}
                <span className="badge polling">
                  <span className="dot" /> Simulated pipeline (submission-svc not running)
                </span>
              </>
            )}
            {source === "live" && (
              <>
                {" · "}
                <span className="badge connected">
                  <span className="dot" /> Live pipeline
                </span>
              </>
            )}
          </div>
        </div>
      </div>

      {!submission && <UploadForm onSubmit={upload} disabled={uploading} />}

      {submission && (
        <>
          <SubmissionStatus submission={submission} />
          <LogViewer logs={logs} />
          {(submission.status === "ready" || submission.status === "failed") && (
            <button type="button" className="primary" onClick={reset} style={{ marginTop: 12 }}>
              Submit another
            </button>
          )}
        </>
      )}
    </main>
  );
}
