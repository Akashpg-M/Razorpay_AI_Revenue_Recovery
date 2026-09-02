import Link from "next/link";
import { Nav } from "@/components/nav";
import { backendJSON } from "@/lib/api";

type Recovery = { case_id:string; merchant_id:string; leak_type:string; amount_at_risk_minor:number; recovered_amount_minor:number; current_state:string; last_action:string; expected_nerv_minor:number; policy_state:string; created_at:string; recovery_deadline:string; attribution_status:string };
type Dashboard = { operational?: { cases?: Recovery[] } };
const money=(n=0)=>new Intl.NumberFormat("en-IN",{style:"currency",currency:"INR",maximumFractionDigits:0}).format(n/100);

export default async function Recoveries(){
  const data=await backendJSON<Dashboard>("/api/v1/dashboard");
  const cases=Array.isArray(data?.operational?.cases)?data.operational.cases:[];
  return <><Nav/><main className="shell"><section className="pageIntro"><div className="eyebrow">LIVE / OPERATIONAL</div><h1>Every recovery.<br/><em>One accountable record.</em></h1><p>Open a case to inspect its risk source, AI ranking, economic gate, policy controls, execution evidence, attribution, and immutable replay.</p></section>
  <section className="caseTable recoveryTable"><div className="tr header"><span>Case / merchant</span><span>Risk</span><span>Decision</span><span>State</span><span></span></div>{cases.map(item=><div className="tr" key={item.case_id}><span><b>{item.case_id.slice(0,14)}</b><small>{item.merchant_id}</small></span><span><b>{money(item.amount_at_risk_minor)}</b><small>{item.leak_type.replaceAll("_"," ")}</small></span><span><b>{item.last_action?.replaceAll("_"," ")||"Not decided"}</b><small>NERV {money(item.expected_nerv_minor)} · {item.policy_state}</small></span><span><i className="statusDot"/>{item.current_state.replaceAll("_"," ")}<small>{item.attribution_status}</small></span><Link href={`/recovery/${item.case_id}`}>Inspect →</Link></div>)}</section>
  {!cases.length&&<div className="empty panel">No cases yet. Launch one from the Scenario Lab.</div>}</main></>;
}
