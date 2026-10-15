import { AuthForm } from "@/components/auth-form";
import Link from "next/link";

export default function SignInPage() {
  return (
    <main className="auth-page">
      <header className="auth-header">
        <Link className="wordmark" href="/">skyroom</Link>
        <span className="status-label">Private beta</span>
      </header>
      <section className="auth-layout">
        <div className="auth-intro">
          <p className="eyebrow">One identity, every Community</p>
          <h1>Come on in.</h1>
          <p>Use any email you can verify. No password, download, or social account required.</p>
        </div>
        <AuthForm />
      </section>
    </main>
  );
}
