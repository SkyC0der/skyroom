import { InvitationJoin } from "@/components/invitation-join";
import Link from "next/link";
import { Suspense } from "react";

export default function InvitationPage() {
  return (
    <main className="auth-page">
      <header className="auth-header">
        <Link className="wordmark" href="/">skyroom</Link>
        <span className="status-label">Community invitation</span>
      </header>
      <section className="auth-layout invitation-layout">
        <div className="auth-intro">
          <p className="eyebrow">Your room is waiting</p>
          <h1>Join the conversation.</h1>
        </div>
        <Suspense fallback={<section className="auth-card"><p className="auth-step">Checking invitation</p><h2>One moment.</h2></section>}>
          <InvitationJoin />
        </Suspense>
      </section>
    </main>
  );
}
