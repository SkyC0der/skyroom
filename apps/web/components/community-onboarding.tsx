"use client";

import { FormEvent, useEffect, useState } from "react";
import { RoomConversation } from "@/components/room-conversation";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

type Community = {
  id: string;
  owner_user_id: string;
  general_room_id: string;
  name: string;
  description: string;
  member_count: number;
};

type Invitation = {
  id: string;
  community_id: string;
  token: string;
  expires_at: string;
  max_uses: number | null;
};

export function CommunityOnboarding({ userID, userHandle, onSignOut, busy: signingOut }: { userID: string; userHandle: string; onSignOut: () => Promise<void>; busy: boolean }) {
  const [communities, setCommunities] = useState<Community[]>([]);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [maxUses, setMaxUses] = useState("");
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [selectedCommunityID, setSelectedCommunityID] = useState("");

  useEffect(() => {
    let active = true;
    async function loadCommunities() {
      try {
        const response = await fetch(`${apiBase}/api/v1/communities`, { credentials: "include" });
        if (!response.ok) throw new Error("load failed");
        const body = (await response.json()) as { communities: Community[] };
        if (active) {
          setCommunities(body.communities);
          setSelectedCommunityID((current) => current || body.communities[0]?.id || "");
        }
      } catch {
        if (active) setError("Skyroom could not load your Communities.");
      } finally {
        if (active) setLoading(false);
      }
    }
    void loadCommunities();
    return () => { active = false; };
  }, []);

  async function createCommunity(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await fetch(`${apiBase}/api/v1/communities`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, description }),
      });
      if (!response.ok) {
        const body = (await response.json()) as { error?: string };
        setError(body.error === "owned_community_limit" ? "Beta accounts can own up to three Communities." : "Check the Community name and try again.");
        return;
      }
      const body = (await response.json()) as { community: Community };
      setCommunities((current) => [...current, body.community]);
      setSelectedCommunityID(body.community.id);
      setName("");
      setDescription("");
      setInvitation(null);
      setNotice(`${body.community.name} is ready with its members-only general Room.`);
    } catch {
      setError("Skyroom could not create the Community. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  async function createInvitation(communityID: string) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const parsedMaxUses = maxUses === "" ? null : Number(maxUses);
      const response = await fetch(`${apiBase}/api/v1/communities/${communityID}/invitations`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ max_uses: parsedMaxUses }),
      });
      if (!response.ok) {
        setError("Skyroom could not create this invitation.");
        return;
      }
      setInvitation((await response.json()) as Invitation);
      setNotice("Invitation created. It expires in seven days.");
    } catch {
      setError("Skyroom could not create this invitation. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  }

  async function revokeInvitation() {
    if (!invitation) return;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/communities/${invitation.community_id}/invitations/${invitation.id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!response.ok) throw new Error("revoke failed");
      setInvitation(null);
      setNotice("Invitation revoked.");
    } catch {
      setError("Skyroom could not revoke this invitation.");
    } finally {
      setBusy(false);
    }
  }

  const inviteURL = invitation && typeof window !== "undefined" ? `${window.location.origin}/invite?token=${encodeURIComponent(invitation.token)}` : "";
  const selectedCommunity = communities.find((item) => item.id === selectedCommunityID);
  const ownedCommunityCount = communities.filter((item) => item.owner_user_id === userID).length;

  return (
    <section className="auth-card community-card" aria-live="polite">
      <div className="community-card__header">
        <div>
          <p className="auth-step">Signed in as @{userHandle}</p>
          <h2>Your Communities</h2>
        </div>
        <button className="compact-button" type="button" disabled={signingOut} onClick={() => void onSignOut()}>Sign out</button>
      </div>

      {loading && <p className="auth-helper">Loading your Communities…</p>}
      {!loading && communities.length === 0 && <p className="auth-helper">Create your first Community. Skyroom will add Owner, Everyone, and a members-only general Room automatically.</p>}

      <div className="community-list">
        {communities.map((item) => (
          <article className="community-item" key={item.id}>
            <div>
              <strong>{item.name}</strong>
              <span>{item.member_count} {item.member_count === 1 ? "member" : "members"}</span>
            </div>
            {item.description && <p>{item.description}</p>}
            <button className="room-open" type="button" onClick={() => setSelectedCommunityID(item.id)}>{selectedCommunityID === item.id ? "Viewing #general" : "Open #general"}</button>
            {item.owner_user_id === userID && (
              <div className="invite-controls">
                <label htmlFor={`max-uses-${item.id}`}>Invite use limit <span>(optional)</span></label>
                <input id={`max-uses-${item.id}`} type="number" min={1} max={10000} value={maxUses} onChange={(event) => setMaxUses(event.target.value)} placeholder="Unlimited" />
                <button type="button" disabled={busy} onClick={() => void createInvitation(item.id)}>Create invitation</button>
              </div>
            )}
          </article>
        ))}
      </div>

      {selectedCommunity && <RoomConversation key={selectedCommunity.id} roomID={selectedCommunity.general_room_id} communityName={selectedCommunity.name} userHandle={userHandle} />}

      {invitation && (
        <div className="invite-result">
          <label htmlFor="invite-url">Invitation link</label>
          <input id="invite-url" readOnly value={inviteURL} onFocus={(event) => event.currentTarget.select()} />
          <p>Expires {new Date(invitation.expires_at).toLocaleString()}{invitation.max_uses ? ` · ${invitation.max_uses} uses maximum` : " · Unlimited uses"}</p>
          <button className="auth-back" type="button" disabled={busy} onClick={() => void revokeInvitation()}>Revoke invitation</button>
        </div>
      )}

      <form className="community-create" onSubmit={createCommunity}>
        <h3>Create a Community</h3>
        <div className="field">
          <label htmlFor="community-name">Name</label>
          <input id="community-name" required minLength={2} maxLength={80} value={name} onChange={(event) => setName(event.target.value)} placeholder="Builders Guild" />
        </div>
        <div className="field">
          <label htmlFor="community-description">Description <span>(optional)</span></label>
          <textarea id="community-description" maxLength={500} value={description} onChange={(event) => setDescription(event.target.value)} placeholder="What brings this Community together?" />
        </div>
        <button className="auth-submit" type="submit" disabled={busy || ownedCommunityCount >= 3}>{busy ? "Working…" : ownedCommunityCount >= 3 ? "Three-Community limit reached" : "Create Community"}</button>
      </form>

      {error && <p className="form-error" role="alert">{error}</p>}
      {notice && <p className="form-notice" role="status">{notice}</p>}
    </section>
  );
}
