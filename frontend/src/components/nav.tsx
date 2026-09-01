import Link from "next/link";
export function Nav() { return <header className="nav"><Link href="/" className="brand"><span className="brandMark">R</span><span>RecoverOS</span></Link><nav><Link href="/">Business impact</Link><Link href="/resilience">Resilience lab</Link></nav><span className="mode">RAZORPAY TEST MODE</span></header>; }
