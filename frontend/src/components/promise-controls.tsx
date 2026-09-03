"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { publicBackend } from "@/lib/api";

export function PromiseControls({ promiseID, active }: { promiseID: string; active: boolean }) {
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const router = useRouter();
  async function resolve(outcome: "fulfilled" | "broken") {
    setBusy(outcome); setError("");
    try {
      const response = await fetch(`${publicBackend}/api/v1/demo/promises/${promiseID}/${outcome}`, { method: "POST" });
      const body = await response.json();
      if (!response.ok) throw new Error(body?.error?.message ?? body?.error ?? "Promise simulation failed");
      router.refresh();
    } catch (caught) { setError(caught instanceof Error ? caught.message : "Promise simulation failed"); }
    finally { setBusy(""); }
  }
  if (!active) return null;
  return <div className="promiseControls"><small>DEMO-ONLY OUTCOME SIMULATION · NOT A PROVIDER PAYMENT</small><div><button disabled={Boolean(busy)} onClick={() => resolve("fulfilled")}>{busy === "fulfilled" ? "Recording…" : "Simulate fulfilled"}</button><button disabled={Boolean(busy)} onClick={() => resolve("broken")}>{busy === "broken" ? "Reassessing…" : "Simulate broken → re-decide"}</button></div>{error && <span className="error">{error}</span>}</div>;
}
