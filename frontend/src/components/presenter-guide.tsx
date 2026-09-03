"use client";

import { useState } from "react";

const steps = [["1", "Run", "Create a scenario", "run"], ["2", "Trace", "Follow persisted stages", "pipeline"], ["3", "Explain", "Inspect AI vs rules", "understand"], ["4", "Bound", "Show gate and policy", "understand"], ["5", "Operate", "Approve when required", "run"], ["6", "Execute", "Open Test Payment", "run"], ["7", "Prove", "Inspect attribution", "prove"], ["8", "Evaluate", "Compare held-out strategies", "proof-map"], ["9", "Break", "Run reliability evidence", "proof-map"]];
export function PresenterGuide() {
  const [open, setOpen] = useState(true);
  return <aside className={`presenterGuide ${open ? "open" : "closed"}`}><button onClick={() => setOpen(!open)}>{open ? "Hide" : "Presenter guide"}</button>{open && <><b>9-step judge walkthrough</b>{steps.map(([n, verb, text, anchor]) => <a href={`#${anchor}`} key={n}><i>{n}</i><span><strong>{verb}</strong><small>{text}</small></span></a>)}</>}</aside>;
}
