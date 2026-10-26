"use client";

import { FormEvent, KeyboardEvent, useEffect, useRef, useState } from "react";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

type MessageState = "pending" | "acknowledged" | "failed";
type ConnectionState = "connecting" | "live" | "recovering";

type Message = {
  id: string;
  community_id: string;
  room_id: string;
  author_user_id: string;
  author_handle: string;
  client_idempotency_key: string;
  body: string;
  revision: number;
  created_at: string;
  edited_at?: string;
  deleted_at?: string;
  state: MessageState;
  mutation?: "editing" | "deleting";
};

type ServerMessage = Omit<Message, "state">;

type HistoryPage = {
  messages: ServerMessage[];
  oldest_cursor?: string;
  latest_cursor?: string;
  has_more_before: boolean;
  has_more_after: boolean;
};

type RoomEvent = {
  event_id: string;
  type: "message.created" | "message.updated" | "message.deleted";
  room_id: string;
  cursor: string;
  payload: { message: ServerMessage };
};

function mergeMessages(current: Message[], incoming: ServerMessage[]) {
  const byClientKey = new Map(current.map((message) => [message.client_idempotency_key, message]));
  for (const message of incoming) {
    const existing = byClientKey.get(message.client_idempotency_key);
    if (!existing || existing.state !== "acknowledged" || message.revision >= existing.revision) {
      byClientKey.set(message.client_idempotency_key, { ...message, state: "acknowledged" });
    }
  }
  return [...byClientKey.values()].sort((left, right) => {
    const timeOrder = new Date(left.created_at).getTime() - new Date(right.created_at).getTime();
    return timeOrder || left.id.localeCompare(right.id);
  });
}

function eventsURL(roomID: string) {
  const url = new URL(`/api/v1/rooms/${roomID}/events`, apiBase || window.location.origin);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

export function RoomConversation({ roomID, roomName, communityName, userHandle }: { roomID: string; roomName: string; communityName: string; userHandle: string }) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [oldestCursor, setOldestCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [body, setBody] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [connection, setConnection] = useState<ConnectionState>("connecting");
  const [unreadLiveCount, setUnreadLiveCount] = useState(0);
  const [unreadAtOpen, setUnreadAtOpen] = useState(0);
  const [readBoundaryID, setReadBoundaryID] = useState("");
  const [error, setError] = useState("");
  const [editingID, setEditingID] = useState("");
  const [editBody, setEditBody] = useState("");
  const historyRef = useRef<HTMLDivElement>(null);
  const latestCursorRef = useRef("");
  const markedReadIDRef = useRef("");

  function nearBottom() {
    const history = historyRef.current;
    return !history || history.scrollHeight - history.scrollTop - history.clientHeight < 72;
  }

  function scrollToLatest(behavior: ScrollBehavior = "smooth") {
    const history = historyRef.current;
    if (!history) return;
    history.scrollTo({ top: history.scrollHeight, behavior });
    setUnreadLiveCount(0);
  }

  useEffect(() => {
    let active = true;
    let socket: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let reconnectAttempt = 0;

    function acceptMessages(incoming: ServerMessage[]) {
      if (!active || incoming.length === 0) return;
      setMessages((current) => mergeMessages(current, incoming));
    }

    async function syncRead(messageID: string) {
      if (!messageID || markedReadIDRef.current === messageID) return;
      markedReadIDRef.current = messageID;
      try {
        const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/read-state`, {
          method: "PUT",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ last_message_id: messageID }),
        });
        if (!response.ok) markedReadIDRef.current = "";
      } catch {
        markedReadIDRef.current = "";
      }
    }

    async function recoverAfter(cursor: string) {
      if (!cursor) return;
      setConnection("recovering");
      let after = cursor;
      try {
        do {
          const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages?after=${encodeURIComponent(after)}&limit=100`, { credentials: "include" });
          if (!response.ok) throw new Error("recovery failed");
          const page = (await response.json()) as HistoryPage;
          if (!active) return;
          acceptMessages(page.messages ?? []);
          if (page.latest_cursor) {
            after = page.latest_cursor;
            latestCursorRef.current = page.latest_cursor;
          }
          if (!page.has_more_after || !page.latest_cursor) break;
        } while (active);
        const currentResponse = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages?limit=100`, { credentials: "include" });
        if (currentResponse.ok) {
          const currentPage = (await currentResponse.json()) as HistoryPage;
          if (active) acceptMessages(currentPage.messages ?? []);
        }
        if (active) setConnection("live");
      } catch {
        if (active) setConnection("recovering");
      }
    }

    async function loadHistory(readStateReady: Promise<void>) {
      try {
        const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages`, { credentials: "include" });
        if (!response.ok) throw new Error("history failed");
        const page = (await response.json()) as HistoryPage;
        await readStateReady;
        if (!active) return;
        acceptMessages(page.messages ?? []);
        setOldestCursor(page.oldest_cursor ?? "");
        setHasMore(page.has_more_before);
        if (!latestCursorRef.current) latestCursorRef.current = page.latest_cursor ?? "";
        const latestMessageID = page.messages.at(-1)?.id;
        requestAnimationFrame(() => {
          scrollToLatest("auto");
          if (latestMessageID && document.visibilityState === "visible") void syncRead(latestMessageID);
        });
      } catch {
        if (active) setError("Skyroom could not load this Room’s history.");
      } finally {
        if (active) setLoading(false);
      }
    }

    async function loadReadState() {
      try {
        const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/read-state`, { credentials: "include" });
        if (!response.ok) return;
        const state = (await response.json()) as { last_read_message_id?: string; unread_count: number };
        if (!active) return;
        setReadBoundaryID(state.last_read_message_id ?? "");
        setUnreadAtOpen(state.unread_count);
      } catch {
        // Message delivery remains usable if read-state sync is temporarily unavailable.
      }
    }

    function connect() {
      if (!active) return;
      setConnection(reconnectAttempt === 0 ? "connecting" : "recovering");
      socket = new WebSocket(eventsURL(roomID));
      socket.onopen = () => {
        reconnectAttempt = 0;
        if (latestCursorRef.current) void recoverAfter(latestCursorRef.current);
        else setConnection("live");
      };
      socket.onmessage = (rawEvent) => {
        const event = JSON.parse(rawEvent.data as string) as RoomEvent;
        if (!event.type.startsWith("message.") || event.room_id !== roomID) return;
        const shouldFollow = nearBottom() || event.payload.message.author_handle === userHandle;
        acceptMessages([event.payload.message]);
        if (event.type === "message.created") {
          latestCursorRef.current = event.cursor;
          if (shouldFollow) requestAnimationFrame(() => scrollToLatest());
          else setUnreadLiveCount((count) => count + 1);
        }
      };
      socket.onclose = () => {
        if (!active) return;
        setConnection("recovering");
        const ceiling = Math.min(10_000, 500 * 2 ** reconnectAttempt);
        const delay = Math.round(ceiling * (0.75 + Math.random() * 0.5));
        reconnectAttempt += 1;
        reconnectTimer = setTimeout(connect, delay);
      };
      socket.onerror = () => socket?.close();
    }

    connect();
    const readStateReady = loadReadState();
    void loadHistory(readStateReady);
    return () => {
      active = false;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      socket?.close(1000, "Room changed");
    };
  }, [roomID, userHandle]);

  async function markRead(messageID: string) {
    if (!messageID || markedReadIDRef.current === messageID) return;
    markedReadIDRef.current = messageID;
    try {
      const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/read-state`, {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ last_message_id: messageID }),
      });
      if (!response.ok) markedReadIDRef.current = "";
    } catch {
      markedReadIDRef.current = "";
    }
  }

  function markLatestIfVisible() {
    if (document.visibilityState !== "visible" || !nearBottom()) return;
    const latest = [...messages].reverse().find((message) => message.state === "acknowledged");
    if (latest) void markRead(latest.id);
    setUnreadLiveCount(0);
  }

  async function loadOlder() {
    if (!oldestCursor) return;
    const history = historyRef.current;
    const previousHeight = history?.scrollHeight ?? 0;
    setLoadingOlder(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages?before=${encodeURIComponent(oldestCursor)}`, { credentials: "include" });
      if (!response.ok) throw new Error("history failed");
      const page = (await response.json()) as HistoryPage;
      setMessages((current) => mergeMessages(current, page.messages ?? []));
      setOldestCursor(page.oldest_cursor ?? oldestCursor);
      setHasMore(page.has_more_before);
      requestAnimationFrame(() => {
        if (history) history.scrollTop += history.scrollHeight - previousHeight;
      });
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
      setMessages((current) => mergeMessages(current, [result.message]));
    } catch {
      setMessages((current) => current.map((item) => item.client_idempotency_key === message.client_idempotency_key ? { ...item, state: "failed" } : item));
    }
  }

  function sendMessage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmedBody = body.trim();
    if (!trimmedBody) return;
    const clientKey = window.crypto.randomUUID();
    const pending: Message = {
      id: `pending-${clientKey}`,
      community_id: "",
      room_id: roomID,
      author_user_id: "",
      author_handle: userHandle,
      client_idempotency_key: clientKey,
      body: trimmedBody,
      revision: 1,
      created_at: new Date().toISOString(),
      state: "pending",
    };
    setMessages((current) => [...current, pending]);
    setBody("");
    requestAnimationFrame(() => scrollToLatest());
    void commitMessage(pending);
  }

  function retryMessage(message: Message) {
    const retry = { ...message, state: "pending" as const };
    setMessages((current) => current.map((item) => item.client_idempotency_key === retry.client_idempotency_key ? retry : item));
    void commitMessage(retry);
  }

  function handleComposerKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  }

  function beginEdit(message: Message) {
    setEditingID(message.id);
    setEditBody(message.body);
    setError("");
  }

  async function saveEdit(message: Message) {
    const trimmedBody = editBody.trim();
    if (!trimmedBody || trimmedBody === message.body) {
      setEditingID("");
      return;
    }
    const previous = message;
    const optimistic = { ...message, body: trimmedBody, revision: message.revision + 1, edited_at: new Date().toISOString(), mutation: "editing" as const };
    setMessages((current) => current.map((item) => item.id === message.id ? optimistic : item));
    setEditingID("");
    try {
      const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages/${message.id}`, {
        method: "PATCH",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ body: trimmedBody, expected_revision: message.revision }),
      });
      if (!response.ok) throw new Error("edit failed");
      const result = (await response.json()) as { message: ServerMessage };
      setMessages((current) => mergeMessages(current, [result.message]));
    } catch {
      setMessages((current) => current.map((item) => item.id === message.id && item.mutation === "editing" ? previous : item));
      setError("That Message could not be edited. It may have changed; try again.");
    }
  }

  async function deleteMessage(message: Message) {
    const previous = message;
    const optimistic = { ...message, body: "", revision: message.revision + 1, deleted_at: new Date().toISOString(), mutation: "deleting" as const };
    setMessages((current) => current.map((item) => item.id === message.id ? optimistic : item));
    setEditingID("");
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/rooms/${roomID}/messages/${message.id}`, {
        method: "DELETE",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ expected_revision: message.revision }),
      });
      if (!response.ok) throw new Error("delete failed");
      const result = (await response.json()) as { message: ServerMessage };
      setMessages((current) => mergeMessages(current, [result.message]));
    } catch {
      setMessages((current) => current.map((item) => item.id === message.id && item.mutation === "deleting" ? previous : item));
      setError("That Message could not be deleted. It may have changed; try again.");
    }
  }

  const readBoundaryIndex = readBoundaryID ? messages.findIndex((message) => message.id === readBoundaryID) : -1;
  const firstUnreadIndex = unreadAtOpen > 0 ? Math.max(0, readBoundaryIndex + 1) : -1;

  return (
    <section className="room-conversation" aria-label={`${communityName} ${roomName} Room`}>
      <header className="room-conversation__header">
        <div><span>{communityName}</span><h3># {roomName}</h3></div>
        <div className={`room-presence room-presence--${connection}`} role="status"><i aria-hidden="true" />{connection === "live" ? "Live" : connection === "connecting" ? "Connecting" : "Recovering"}</div>
      </header>
      <div className="message-history" ref={historyRef} aria-live="polite" onScroll={markLatestIfVisible}>
        {hasMore && <button className="load-older" type="button" disabled={loadingOlder} onClick={() => void loadOlder()}>{loadingOlder ? "Loading…" : "Load older Messages"}</button>}
        {loading && <p className="empty-history">Loading Messages…</p>}
        {!loading && messages.length === 0 && <p className="empty-history">No Messages yet. Start the conversation.</p>}
        {messages.map((message, index) => (
          <article className={`room-message room-message--${message.state}`} key={message.client_idempotency_key}>
            {index === firstUnreadIndex && <div className="unread-divider"><span>{unreadAtOpen} unread when you opened</span></div>}
            <div className="room-message__meta">
              <strong>@{message.author_handle}</strong>
              <time dateTime={message.created_at}>{new Date(message.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>
              {message.state !== "acknowledged" && <span>{message.state === "pending" ? "Sending…" : "Not sent"}</span>}
              {message.edited_at && !message.deleted_at && <span>Edited</span>}
            </div>
            {message.deleted_at ? <p className="room-message__tombstone">Message deleted</p> : editingID === message.id ? (
              <form className="message-edit" onSubmit={(event) => { event.preventDefault(); void saveEdit(message); }}>
                <textarea required maxLength={4000} value={editBody} onChange={(event) => setEditBody(event.target.value)} autoFocus />
                <div><button type="submit" disabled={!editBody.trim()}>Save</button><button type="button" onClick={() => setEditingID("")}>Cancel</button></div>
              </form>
            ) : <p>{message.body}</p>}
            {message.state === "acknowledged" && !message.deleted_at && message.author_handle === userHandle && editingID !== message.id && (
              <div className="message-actions"><button type="button" disabled={Boolean(message.mutation)} onClick={() => beginEdit(message)}>Edit</button><button type="button" disabled={Boolean(message.mutation)} onClick={() => void deleteMessage(message)}>Delete</button></div>
            )}
            {message.state === "failed" && <button className="retry-message" type="button" onClick={() => retryMessage(message)}>Retry</button>}
          </article>
        ))}
      </div>
      {unreadLiveCount > 0 && <button className="new-message-jump" type="button" onClick={() => scrollToLatest()}>{unreadLiveCount} new {unreadLiveCount === 1 ? "Message" : "Messages"} ↓</button>}
      {error && <p className="form-error room-error" role="alert">{error}</p>}
      <form className="message-composer" onSubmit={sendMessage}>
        <label htmlFor={`message-${roomID}`}>Message #{roomName}</label>
        <textarea id={`message-${roomID}`} required maxLength={4000} value={body} onChange={(event) => setBody(event.target.value)} onKeyDown={handleComposerKeyDown} placeholder={`Message ${communityName}`} />
        <div><span>{body.length}/4000 · Enter to send · Shift + Enter for a new line</span><button type="submit" disabled={!body.trim()}>Send</button></div>
      </form>
    </section>
  );
}
