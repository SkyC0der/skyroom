"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

type Preview = {
  community_name: string;
  community_description: string;
  member_count: number;
  inviter_handle: string;
  expires_at: string;
};

export function InvitationJoin() {
  const token = useSearchParams().get("token") ?? "";
  const [preview, setPreview] = useState<Preview | null>(null);
  const [signedIn, setSignedIn] = useState(false);
  const [joined, setJoined] = useState(false);
  const [loading, setLoading] = useState(token !== "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(token === "" ? "This invitation could not be found." : "");

  useEffect(() => {
    let active = true;
    if (!token) return;
    window.localStorage.setItem("pending_invitation_token", token);
    async function load() {
      try {
        const [previewResponse, sessionResponse] = await Promise.all([
          fetch(`${apiBase}/api/v1/invitations/${token}`),
          fetch(`${apiBase}/api/v1/auth/session`, { credentials: "include" }),
        ]);
        if (!active) return;
        if (!previewResponse.ok) {
          setError(previewResponse.status === 410 ? "This invitation is no longer active." : "This invitation could not be found.");
          return;
        }
        setPreview((await previewResponse.json()) as Preview);
        if (sessionResponse.ok) {
          const session = (await sessionResponse.json()) as { needs_handle: boolean };
          setSignedIn(!session.needs_handle);
        } else {
          setSignedIn(false);
        }
      } catch {
        if (active) setError("Skyroom could not load this invitation.");
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    return () => { active = false; };
  }, [token]);

  async function join() {
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/invitations/${token}/join`, { method: "POST", credentials: "include" });
      if (!response.ok) {
        if (response.status === 401) setSignedIn(false);
        else if (response.status === 410) setError("This invitation is no longer active.");
        else setError("Skyroom could not join this Community.");
        return;
      }
      window.localStorage.removeItem("pending_invitation_token");
      setJoined(true);
    } catch {
      setError("Skyroom could not join this Community. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <section className="auth-card"><p className="auth-step">Checking invitation</p><h2>One moment.</h2></section>;
  if (!preview) return <section className="auth-card"><p className="auth-step">Invitation unavailable</p><h2>That link won&apos;t work.</h2>{error && <p className="form-error">{error}</p>}</section>;
  if (joined) return <section className="auth-card auth-card--complete"><span className="auth-check">✓</span><p className="auth-step">Membership active</p><h2>You joined {preview.community_name}.</h2><p>The members-only general Room is ready for you.</p><Link className="primary-action" href="/sign-in">View your Communities</Link></section>;

  return (
    <section className="auth-card invitation-card">
      <p className="auth-step">Invited by @{preview.inviter_handle}</p>
      <h2>{preview.community_name}</h2>
      <p className="auth-helper">{preview.community_description || "A private Skyroom Community."}</p>
      <dl>
        <div><dt>Members</dt><dd>{preview.member_count}</dd></div>
        <div><dt>Invitation expires</dt><dd>{new Date(preview.expires_at).toLocaleDateString()}</dd></div>
      </dl>
      {error && <p className="form-error" role="alert">{error}</p>}
      {signedIn ? (
        <button className="auth-submit" type="button" disabled={busy} onClick={() => void join()}>{busy ? "Joining…" : "Join Community"}</button>
      ) : (
        <Link className="primary-action invitation-sign-in" href="/sign-in">Sign in to join</Link>
      )}
    </section>
  );
}
