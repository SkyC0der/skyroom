"use client";

import { FormEvent, useState } from "react";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

type Step = "email" | "code" | "complete";

export function AuthForm() {
  const [step, setStep] = useState<Step>("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function requestCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/auth/codes`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
      });
      if (!response.ok) {
        setError(response.status === 429 ? "A code was sent recently. Wait one minute and try again." : "We could not send a code. Check the email and try again.");
        return;
      }
      setStep("code");
    } catch {
      setError("Skyroom could not reach the API. Check your connection and try again.");
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
      setStep("complete");
    } catch {
      setError("Skyroom could not verify the code. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  if (step === "complete") {
    return (
      <section className="auth-card auth-card--complete" aria-live="polite">
        <span className="auth-check" aria-hidden="true">✓</span>
        <p className="auth-step">Email verified</p>
        <h2>You&apos;re signed in.</h2>
        <p>Your session is active. Choosing a unique Handle is the next implementation ticket.</p>
      </section>
    );
  }

  return (
    <form className="auth-card" onSubmit={step === "email" ? requestCode : verifyCode}>
      <p className="auth-step">Step {step === "email" ? "1" : "2"} of 2</p>
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
      <button className="auth-submit" type="submit" disabled={busy}>{busy ? "Working…" : step === "email" ? "Send verification code" : "Verify and continue"}</button>
      {step === "code" && <button className="auth-back" type="button" onClick={() => { setStep("email"); setCode(""); setError(""); }}>Use a different email</button>}
      <p className="auth-legal">By continuing, you agree to the beta terms and acknowledge the privacy notice.</p>
    </form>
  );
}
