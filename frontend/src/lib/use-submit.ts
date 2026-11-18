"use client";

import { useState } from "react";
import { ApiError, errorMessage } from "./api";

// useSubmit runs a save action and keeps the API's message and per-field
// errors so a form can show them next to the right inputs.
export function useSubmit() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fields, setFields] = useState<Record<string, string> | undefined>();

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError(null);
    setFields(undefined);
    try {
      await action();
    } catch (err) {
      setFields(err instanceof ApiError ? err.fields : undefined);
      // When every problem is attached to a field, the summary would only repeat them.
      setError(err instanceof ApiError && err.fields && err.code === "VALIDATION_FAILED" ? "Check the highlighted fields." : errorMessage(err));
    }
    setBusy(false);
  }

  return { busy, error, fields, run };
}
