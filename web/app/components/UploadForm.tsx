"use client";

import { useState } from "react";

type Props = {
  onSubmit: (args: { file: File; lang: string; entrypoint: string; contestantID: string }) => void;
  disabled?: boolean;
};

export function UploadForm({ onSubmit, disabled }: Props) {
  const [file, setFile] = useState<File | null>(null);
  const [lang, setLang] = useState("go");
  const [entrypoint, setEntrypoint] = useState(".");
  const [contestantID, setContestantID] = useState("");

  const canSubmit = !!file && contestantID.trim().length > 0 && !disabled;

  return (
    <form
      className="upload-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (!file || !canSubmit) return;
        onSubmit({ file, lang, entrypoint, contestantID });
      }}
    >
      <div className="field">
        <label htmlFor="contestant_id">Contestant ID</label>
        <input
          id="contestant_id"
          type="text"
          value={contestantID}
          placeholder="e.g. team-alpha"
          onChange={(e) => setContestantID(e.target.value)}
          required
        />
      </div>

      <div className="field-row">
        <div className="field">
          <label htmlFor="lang">Language</label>
          <select id="lang" value={lang} onChange={(e) => setLang(e.target.value)}>
            <option value="go">Go</option>
            <option value="rust">Rust</option>
            <option value="cpp">C++</option>
          </select>
        </div>
        <div className="field" style={{ flex: 1 }}>
          <label htmlFor="entrypoint">Entrypoint</label>
          <input
            id="entrypoint"
            type="text"
            value={entrypoint}
            onChange={(e) => setEntrypoint(e.target.value)}
            placeholder="."
          />
        </div>
      </div>

      <div className="field">
        <label htmlFor="file">Source archive (.tar.gz, max 50 MB)</label>
        <input
          id="file"
          type="file"
          accept=".tar.gz,.tgz,application/gzip,application/x-tar"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          required
        />
        {file && (
          <div className="file-meta">
            {file.name} · {(file.size / 1024).toFixed(1)} KB
          </div>
        )}
      </div>

      <button type="submit" disabled={!canSubmit} className="primary">
        {disabled ? "Uploading…" : "Submit for benchmarking"}
      </button>
    </form>
  );
}
