"use client";

import { FormEvent, useEffect, useState } from "react";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

type MessageState = "pending" | "acknowledged" | "failed";

type Message = {
  id: string;
  room_id: string;
  author_handle: string;
  client_idempotency_key: string;
  body: string;
  created_at: string;
  state: MessageState;
};

type ServerMessage = Omit<Message, "state">;

type HistoryPage = {
  messages: ServerMessage[];
  oldest_cursor?: string;
  has_more_before: boolean;
};

export function RoomConversation({ roomID, communityName, userHandle }: { roomID: string; communityName: string; userHandle: string }) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [oldestCursor, setOldestCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [body, setBody] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    async function loadHistory() {
      try {
        const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages`, { credentials: "include" });
        if (!response.ok) throw new Error("history failed");
        const page = (await response.json()) as HistoryPage;
        if (!active) return;
        const acknowledged = (page.messages ?? []).map((message) => ({ ...message, state: "acknowledged" as const }));
        setMessages((current) => {
          const persistedKeys = new Set(acknowledged.map((message) => message.client_idempotency_key));
          return [...acknowledged, ...current.filter((message) => !persistedKeys.has(message.client_idempotency_key))];
        });
        setOldestCursor(page.oldest_cursor ?? "");
        setHasMore(page.has_more_before);
      } catch {
        if (active) setError("Skyroom could not load this Room’s history.");
      } finally {
        if (active) setLoading(false);
      }
    }
    void loadHistory();
    return () => { active = false; };
  }, [roomID]);

  async function loadOlder() {
    if (!oldestCursor) return;
    setLoadingOlder(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages?before=${encodeURIComponent(oldestCursor)}`, { credentials: "include" });
      if (!response.ok) throw new Error("history failed");
      const page = (await response.json()) as HistoryPage;
      setMessages((current) => {
        const known = new Set(current.map((message) => message.id));
        return [...page.messages.filter((message) => !known.has(message.id)).map((message) => ({ ...message, state: "acknowledged" as const })), ...current];
      });
      setOldestCursor(page.oldest_cursor ?? oldestCursor);
      setHasMore(page.has_more_before);
    } catch {
      setError("Skyroom could not load older Messages.");
    } finally {
      setLoadingOlder(false);
    }
  }

  async function commitMessage(message: Message) {
    try {
      const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ client_idempotency_key: message.client_idempotency_key, body: message.body }),
      });
      if (!response.ok) throw new Error("send failed");
      const result = (await response.json()) as { message: ServerMessage };
      setMessages((current) => current.map((item) => item.client_idempotency_key === message.client_idempotency_key ? { ...result.message, state: "acknowledged" } : item));
    } catch {
      setMessages((current) => current.map((item) => item.client_idempotency_key === message.client_idempotency_key ? { ...item, state: "failed" } : item));
    }
  }

  function sendMessage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!body.trim()) return;
    const clientKey = window.crypto.randomUUID();
    const pending: Message = {
      id: `pending-${clientKey}`,
      room_id: roomID,
      author_handle: userHandle,
      client_idempotency_key: clientKey,
      body,
      created_at: new Date().toISOString(),
      state: "pending",
    };
    setMessages((current) => [...current, pending]);
    setBody("");
    void commitMessage(pending);
  }

  function retryMessage(message: Message) {
    const retry = { ...message, state: "pending" as const };
    setMessages((current) => current.map((item) => item.client_idempotency_key === retry.client_idempotency_key ? retry : item));
    void commitMessage(retry);
  }

  return (
    <section className="room-conversation" aria-label={`${communityName} general Room`}>
      <header className="room-conversation__header">
        <div><span>Room</span><h3># general</h3></div>
        <strong>{communityName}</strong>
      </header>
      <div className="message-history" aria-live="polite">
        {hasMore && <button className="load-older" type="button" disabled={loadingOlder} onClick={() => void loadOlder()}>{loadingOlder ? "Loading…" : "Load older Messages"}</button>}
        {loading && <p className="empty-history">Loading Messages…</p>}
        {!loading && messages.length === 0 && <p className="empty-history">No Messages yet. Start the conversation.</p>}
        {messages.map((message) => (
          <article className={`room-message room-message--${message.state}`} key={message.client_idempotency_key}>
            <div className="room-message__meta">
              <strong>@{message.author_handle}</strong>
              <time dateTime={message.created_at}>{new Date(message.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>
              <span>{message.state === "acknowledged" ? "Acknowledged" : message.state === "pending" ? "Pending…" : "Failed"}</span>
            </div>
            <p>{message.body}</p>
            {message.state === "failed" && <button className="retry-message" type="button" onClick={() => retryMessage(message)}>Retry with the same key</button>}
          </article>
        ))}
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      <form className="message-composer" onSubmit={sendMessage}>
        <label htmlFor={`message-${roomID}`}>Message #general</label>
        <textarea id={`message-${roomID}`} required maxLength={4000} value={body} onChange={(event) => setBody(event.target.value)} placeholder={`Message ${communityName}`} />
        <div><span>{body.length}/4000</span><button type="submit" disabled={!body.trim()}>Send Message</button></div>
      </form>
    </section>
  );
}
