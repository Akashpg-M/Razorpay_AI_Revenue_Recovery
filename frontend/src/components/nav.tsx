import Link from "next/link";
import { backendJSON } from "@/lib/api";

type ProviderStatus = { selected_provider?: string; mode?: string; authenticated?: boolean };

export async function Nav() {
  const provider = await backendJSON<ProviderStatus>("/api/v1/integrations/razorpay/status");
  const razorpay = provider?.selected_provider === "razorpay";
  const mode = razorpay ? `RAZORPAY ${provider?.mode?.toUpperCase() ?? "TEST"} MODE` : "LOCAL DEMO MODE";
  return <header className="nav">
    <Link href="/" className="brand"><span className="brandMark">R</span><span>RecoverOS</span></Link>
    <nav>
      <Link href="/">Command Center</Link><Link href="/recoveries">Recoveries</Link>
      <Link href="/operations">Operations</Link><Link href="/evaluation">Evaluation</Link>
      <Link href="/resilience">Reliability</Link><Link href="/observability">Observability</Link>
      <Link href="/demo">Demo</Link>
    </nav>
    <span className={`mode ${razorpay ? "razorpay" : "local"}`}>{mode}{razorpay && provider?.authenticated ? " · CONNECTED" : ""}</span>
  </header>;
}
