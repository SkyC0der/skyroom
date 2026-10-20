"use client";

import { AuthForm } from "@/components/auth-form";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

function RoomEntry() {
  const roomID = useSearchParams().get("room") ?? undefined;
  return <AuthForm initialRoomID={roomID} />;
}

export default function RoomPage() {
  return (
    <main className="auth-page workspace-page">
      <header className="auth-header">
        <Link className="wordmark" href="/">skyroom</Link>
        <span className="status-label">Private beta</span>
      </header>
      <section className="auth-layout workspace-auth-layout">
        <div className="auth-intro workspace-intro">
          <p className="eyebrow">One identity, every Community</p>
          <h1>Come on in.</h1>
          <p>Sign in to open this Room. Access is checked against your active memberships.</p>
        </div>
        <Suspense fallback={<section className="auth-card"><p className="auth-step">Opening Room</p><h2>One moment.</h2></section>}>
          <RoomEntry />
        </Suspense>
      </section>
    </main>
  );
}
