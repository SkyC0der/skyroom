import { ServiceStatus } from "@/components/service-status";
import Link from "next/link";

export default function HomePage() {
  return (
    <main>
      <nav aria-label="Primary navigation">
        <Link className="wordmark" href="/">
          skyroom
        </Link>
        <span className="status-label">Private preview</span>
      </nav>

      <section className="hero">
        <p className="eyebrow">Communities, without the installation</p>
        <h1>The room where your community keeps talking.</h1>
        <p className="lede">
          Join with your email, find the people and rooms that matter, and move from text to live conversation in one browser tab.
        </p>

        <div className="actions">
          <Link className="primary-action" href="/sign-in">Enter the private beta</Link>
          <ServiceStatus />
        </div>
      </section>

      <section className="principles" aria-label="Product principles">
        <article>
          <span>01</span>
          <h2>Arrive quickly</h2>
          <p>A verified email and a global handle are all a member needs.</p>
        </article>
        <article>
          <span>02</span>
          <h2>Stay in context</h2>
          <p>Rooms preserve the questions, people, and decisions around every community.</p>
        </article>
        <article>
          <span>03</span>
          <h2>Speak when it matters</h2>
          <p>Move into voice or video without leaving the conversation.</p>
        </article>
      </section>
    </main>
  );
}
