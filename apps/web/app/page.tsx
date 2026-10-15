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
        <p className="eyebrow">Conference conversation, without the installation</p>
        <h1>The room where your event keeps talking.</h1>
        <p className="lede">
          Join with your work email, find the people and rooms that matter, and move from text to live conversation in one browser tab.
        </p>

        <div className="actions">
          <button type="button" disabled>
            Request early access
          </button>
          <ServiceStatus />
        </div>
      </section>

      <section className="principles" aria-label="Product principles">
        <article>
          <span>01</span>
          <h2>Arrive quickly</h2>
          <p>A verified work email and a username are all a member needs.</p>
        </article>
        <article>
          <span>02</span>
          <h2>Stay in context</h2>
          <p>Rooms preserve the questions, people, and decisions around every session.</p>
        </article>
        <article>
          <span>03</span>
          <h2>Speak when it matters</h2>
          <p>Move into voice, video, or a moderated stage without leaving the community.</p>
        </article>
      </section>
    </main>
  );
}
