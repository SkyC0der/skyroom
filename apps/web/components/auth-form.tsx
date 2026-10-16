"use client";

import { FormEvent, useEffect, useState } from "react";
import { CommunityOnboarding } from "@/components/community-onboarding";
import { useRouter } from "next/navigation";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

type Step = "loading" | "email" | "code" | "handle" | "complete";
type User = { id: string; email: string; handle: string | null };
type AuthResponse = { user: User; needs_handle: boolean };

export function AuthForm() {
  const router = useRouter();
  const [step, setStep] = useState<Step>("loading");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [handle, setHandle] = useState("");
  const [user, setUser] = useState<User | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [resendAfter, setResendAfter] = useState(0);

  useEffect(() => {
    let active = true;
    async function restoreSession() {
      try {
        const response = await fetch(`${apiBase}/api/v1/auth/session`, { credentials: "include" });
        if (!active) return;
        if (response.status === 401) {
          setStep("email");
          return;
        }
        if (!response.ok) throw new Error("session lookup failed");
        const authenticated = (await response.json()) as AuthResponse;
        setUser(authenticated.user);
        setEmail(authenticated.user.email);
        setStep(authenticated.needs_handle ? "handle" : "complete");
      } catch {
        if (active) {
          setError("Skyroom could not reach the API. Check your connection and try again.");
          setStep("email");
        }
      }
    }
    void restoreSession();
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (step !== "code" || resendAfter <= 0) return;
    const timer = window.setTimeout(() => setResendAfter((seconds) => Math.max(0, seconds - 1)), 1000);
    return () => window.clearTimeout(timer);
  }, [resendAfter, step]);

  useEffect(() => {
    if (step !== "complete") return;
    const pendingInvitation = window.localStorage.getItem("pending_invitation_token");
    if (pendingInvitation) {
      window.localStorage.removeItem("pending_invitation_token");
      router.replace(`/invite?token=${encodeURIComponent(pendingInvitation)}`);
    }
  }, [router, step]);

  async function sendVerificationCode(): Promise<"sent" | "throttled" | "failed"> {
    const response = await fetch(`${apiBase}/api/v1/auth/codes`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
    if (!response.ok) {
      if (response.status === 429) {
        const retryAfter = Number(response.headers.get("Retry-After"));
        setResendAfter(Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : 60);
        setError("A code was already sent. Enter it below or wait for the countdown to resend.");
        return "throttled";
      } else {
        setError("We could not send a code. Check the email and try again.");
      }
      return "failed";
    }
    setResendAfter(60);
    return "sent";
  }

  async function requestCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    try {
      if (await sendVerificationCode() !== "failed") setStep("code");
    } catch {
      setError("Skyroom could not reach the API. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  async function resendCode() {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      if (await sendVerificationCode() === "sent") {
        setCode("");
        setNotice("A new verification code was sent.");
      }
    } catch {
      setError("Skyroom could not resend the code. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  async function verifyCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/auth/verify`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code }),
      });
      if (!response.ok) {
        setError(response.status === 429 ? "Too many attempts. Request a new code." : "That code is invalid or expired. Check it and try again.");
        return;
      }
      const authenticated = (await response.json()) as AuthResponse;
      setUser(authenticated.user);
      setStep(authenticated.needs_handle ? "handle" : "complete");
    } catch {
      setError("Skyroom could not verify the code. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  async function claimHandle(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/users/me/handle`, {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ handle }),
      });
      if (!response.ok) {
        if (response.status === 409) setError("That handle is already taken. Try another.");
        else if (response.status === 400) setError("Use 3–32 letters, numbers, underscores, or periods.");
        else if (response.status === 403) {
          setError("Verify your email again before choosing a handle.");
          setStep("email");
          setCode("");
        } else if (response.status === 401) setError("Your session expired. Verify your email again.");
        else setError("Skyroom could not save your handle. Try again.");
        return;
      }
      const authenticated = (await response.json()) as AuthResponse;
      setUser(authenticated.user);
      setStep("complete");
    } catch {
      setError("Skyroom could not save your handle. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  async function signOut() {
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/auth/session`, { method: "DELETE", credentials: "include" });
      if (!response.ok) throw new Error("sign out failed");
      setUser(null);
      setCode("");
      setHandle("");
      setStep("email");
    } catch {
      setError("Skyroom could not sign you out. Try again.");
    } finally {
      setBusy(false);
    }
  }

  if (step === "loading") {
    return <section className="auth-card" aria-live="polite"><p className="auth-step">Checking session</p><h2>Welcome back.</h2></section>;
  }

  if (step === "complete") {
    return <CommunityOnboarding userID={user?.id ?? ""} userHandle={user?.handle ?? ""} onSignOut={signOut} busy={busy} />;
  }

  if (step === "handle") {
    return (
      <form className="auth-card" onSubmit={claimHandle}>
        <p className="auth-step">Step 3 of 3</p>
        <h2>Choose your Handle</h2>
        <p className="auth-helper">This unique name identifies you across Skyroom. You can change it later.</p>
        <div className="field">
          <label htmlFor="handle">Global Handle</label>
          <input id="handle" name="handle" type="text" autoComplete="username" required minLength={3} maxLength={32} pattern="[A-Za-z0-9][A-Za-z0-9._]{1,30}[A-Za-z0-9]" value={handle} onChange={(event) => setHandle(event.target.value)} placeholder="sky.member" autoFocus />
        </div>
        {error && <p className="form-error" role="alert">{error}</p>}
        <button className="auth-submit" type="submit" disabled={busy}>{busy ? "Saving…" : "Claim Handle"}</button>
        <p className="auth-legal">Letters are stored lowercase. Periods and underscores cannot begin or end a Handle.</p>
      </form>
    );
  }

  return (
    <form className="auth-card" onSubmit={step === "email" ? requestCode : verifyCode}>
      <p className="auth-step">Step {step === "email" ? "1" : "2"} of 3</p>
      <h2>{step === "email" ? "Verify your email" : "Enter the six-digit code"}</h2>
      <p className="auth-helper">
        {step === "email" ? "We’ll send a short-lived sign-in code." : <>Sent to <strong>{email}</strong>.</>}
      </p>

      {step === "email" ? (
        <div className="field">
          <label htmlFor="email">Email address</label>
          <input id="email" name="email" type="email" autoComplete="email" required maxLength={254} value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@example.com" />
        </div>
      ) : (
        <div className="field">
          <label htmlFor="code">Verification code</label>
          <input id="code" name="code" type="text" inputMode="numeric" autoComplete="one-time-code" required pattern="[0-9]{6}" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))} placeholder="000000" className="code-input" autoFocus />
        </div>
      )}

      {error && <p className="form-error" role="alert">{error}</p>}
      {notice && <p className="form-notice" role="status">{notice}</p>}
      <button className="auth-submit" type="submit" disabled={busy}>{busy ? "Working…" : step === "email" ? "Send verification code" : "Verify and continue"}</button>
      {step === "code" && (
        <>
          <button className="auth-back" type="button" disabled={busy || resendAfter > 0} onClick={() => void resendCode()}>
            {resendAfter > 0 ? `Resend code in ${resendAfter}s` : "Resend code"}
          </button>
          <button className="auth-back" type="button" disabled={busy} onClick={() => { setStep("email"); setCode(""); setError(""); setNotice(""); setResendAfter(0); }}>Use a different email</button>
        </>
      )}
      <p className="auth-legal">By continuing, you agree to the beta terms and acknowledge the privacy notice.</p>
    </form>
  );
}
