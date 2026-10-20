"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
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

type Room = {
  id: string;
  community_id: string;
  name: string;
  kind: "text";
  visibility: "members";
};

type Invitation = {
  id: string;
  community_id: string;
  token: string;
  expires_at: string;
  max_uses: number | null;
};

type Props = {
  userID: string;
  userHandle: string;
  initialRoomID?: string;
  onSignOut: () => Promise<void>;
  busy: boolean;
};

function roomPath(roomID: string) {
  return `/rooms?room=${encodeURIComponent(roomID)}`;
}

export function CommunityOnboarding({ userID, userHandle, initialRoomID, onSignOut, busy: signingOut }: Props) {
  const [communities, setCommunities] = useState<Community[]>([]);
  const [roomsByCommunity, setRoomsByCommunity] = useState<Record<string, Room[]>>({});
  const [selectedCommunityID, setSelectedCommunityID] = useState("");
  const [selectedRoomID, setSelectedRoomID] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [maxUses, setMaxUses] = useState("");
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const allRooms = useMemo(() => Object.values(roomsByCommunity).flat(), [roomsByCommunity]);
  const selectedCommunity = communities.find((item) => item.id === selectedCommunityID);
  const selectedRoom = allRooms.find((item) => item.id === selectedRoomID);
  const ownedCommunityCount = communities.filter((item) => item.owner_user_id === userID).length;

  useEffect(() => {
    let active = true;
    async function loadWorkspace() {
      try {
        const response = await fetch(`${apiBase}/api/v1/communities`, { credentials: "include" });
        if (!response.ok) throw new Error("load failed");
        const body = (await response.json()) as { communities: Community[] };
        const roomEntries = await Promise.all(body.communities.map(async (community) => {
          const roomResponse = await fetch(`${apiBase}/api/v1/communities/${community.id}/rooms`, { credentials: "include" });
          if (!roomResponse.ok) throw new Error("room load failed");
          const roomBody = (await roomResponse.json()) as { rooms: Room[] };
          return [community.id, roomBody.rooms] as const;
        }));
        if (!active) return;
        const roomMap = Object.fromEntries(roomEntries);
        const linkedRoom = roomEntries.flatMap(([, rooms]) => rooms).find((room) => room.id === initialRoomID);
        const firstCommunity = linkedRoom
          ? body.communities.find((community) => community.id === linkedRoom.community_id)
          : body.communities[0];
        const firstRoom = linkedRoom ?? roomMap[firstCommunity?.id ?? ""]?.[0];
        setCommunities(body.communities);
        setRoomsByCommunity(roomMap);
        setSelectedCommunityID(firstCommunity?.id ?? "");
        setSelectedRoomID(firstRoom?.id ?? "");
        if (initialRoomID && !linkedRoom) setError("That Room is unavailable or outside your Communities.");
      } catch {
        if (active) setError("Skyroom could not load your workspace.");
      } finally {
        if (active) setLoading(false);
      }
    }
    void loadWorkspace();
    return () => { active = false; };
  }, [initialRoomID]);

  useEffect(() => {
    function restoreRoomFromHistory() {
      if (window.location.pathname !== "/rooms") return;
      const roomID = new URLSearchParams(window.location.search).get("room");
      const room = allRooms.find((item) => item.id === roomID);
      if (!room) return;
      setSelectedCommunityID(room.community_id);
      setSelectedRoomID(room.id);
    }
    window.addEventListener("popstate", restoreRoomFromHistory);
    return () => window.removeEventListener("popstate", restoreRoomFromHistory);
  }, [allRooms]);

  function openRoom(room: Room, replace = false) {
    setSelectedCommunityID(room.community_id);
    setSelectedRoomID(room.id);
    setError("");
    setInvitation(null);
    window.history[replace ? "replaceState" : "pushState"]({}, "", roomPath(room.id));
  }

  function openCommunity(communityID: string) {
    const firstRoom = roomsByCommunity[communityID]?.[0];
    setSelectedCommunityID(communityID);
    if (firstRoom) openRoom(firstRoom);
  }

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
      const body = (await response.json()) as { community: Community; general_room: Room };
      setCommunities((current) => [...current, body.community]);
      setRoomsByCommunity((current) => ({ ...current, [body.community.id]: [body.general_room] }));
      setName("");
      setDescription("");
      setNotice(`${body.community.name} is ready.`);
      openRoom(body.general_room);
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
      if (!response.ok) throw new Error("invite failed");
      setInvitation((await response.json()) as Invitation);
      setNotice("Invitation created. It expires in seven days.");
    } catch {
      setError("Skyroom could not create this invitation.");
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

  async function copyRoomLink() {
    if (!selectedRoom) return;
    try {
      await navigator.clipboard.writeText(`${window.location.origin}${roomPath(selectedRoom.id)}`);
      setNotice(`Link to #${selectedRoom.name} copied.`);
    } catch {
      setError("Skyroom could not copy this Room link. Copy it from the address bar instead.");
    }
  }

  const inviteURL = invitation && typeof window !== "undefined" ? `${window.location.origin}/invite?token=${encodeURIComponent(invitation.token)}` : "";

  return (
    <section className="community-card" aria-live="polite">
      <header className="workspace-header">
        <div><span className="workspace-kicker">Skyroom workspace</span><strong>@{userHandle}</strong></div>
        <button className="compact-button" type="button" disabled={signingOut} onClick={() => void onSignOut()}>Sign out</button>
      </header>

      {loading && <div className="workspace-loading">Loading your Communities and Rooms…</div>}

      {!loading && (
        <div className="workspace-shell">
          <nav className="community-rail" aria-label="Communities">
            {communities.map((community) => (
              <button className={selectedCommunityID === community.id ? "community-switch community-switch--active" : "community-switch"} type="button" key={community.id} title={community.name} aria-label={`Open ${community.name}`} aria-current={selectedCommunityID === community.id ? "page" : undefined} onClick={() => openCommunity(community.id)}>
                {community.name.slice(0, 2).toUpperCase()}
              </button>
            ))}
            <span className="community-count">{communities.length}/3</span>
          </nav>

          <aside className="room-navigation">
            {selectedCommunity ? (
              <>
                <div className="room-navigation__heading"><span>Community</span><h2>{selectedCommunity.name}</h2><p>{selectedCommunity.member_count} {selectedCommunity.member_count === 1 ? "member" : "members"}</p></div>
                <div className="room-list" role="list" aria-label={`${selectedCommunity.name} Rooms`}>
                  <span className="room-list__label">Text Rooms</span>
                  {(roomsByCommunity[selectedCommunity.id] ?? []).map((room) => (
                    <button className={selectedRoomID === room.id ? "room-switch room-switch--active" : "room-switch"} type="button" role="listitem" key={room.id} aria-current={selectedRoomID === room.id ? "page" : undefined} onClick={() => openRoom(room)}><span aria-hidden="true">#</span>{room.name}</button>
                  ))}
                </div>
                {selectedCommunity.owner_user_id === userID && (
                  <details className="workspace-tools">
                    <summary>Invite members</summary>
                    <label htmlFor="invite-max-uses">Use limit <span>(optional)</span></label>
                    <input id="invite-max-uses" type="number" min={1} max={10000} value={maxUses} onChange={(event) => setMaxUses(event.target.value)} placeholder="Unlimited" />
                    <button type="button" disabled={busy} onClick={() => void createInvitation(selectedCommunity.id)}>Create invitation</button>
                  </details>
                )}
              </>
            ) : <p className="room-navigation__empty">Create a Community to begin.</p>}
          </aside>

          <div className="room-stage">
            {selectedRoom && selectedCommunity ? (
              <>
                <button className="copy-room-link" type="button" onClick={() => void copyRoomLink()}>Copy Room link</button>
                <RoomConversation key={selectedRoom.id} roomID={selectedRoom.id} roomName={selectedRoom.name} communityName={selectedCommunity.name} userHandle={userHandle} />
              </>
            ) : !loading && <div className="workspace-empty"><span>00</span><h2>No Room selected</h2><p>Create or join a Community to start a conversation.</p></div>}
          </div>
        </div>
      )}

      <footer className="workspace-footer">
        {invitation && (
          <div className="invite-result">
            <label htmlFor="invite-url">Invitation link</label>
            <input id="invite-url" readOnly value={inviteURL} onFocus={(event) => event.currentTarget.select()} />
            <p>Expires {new Date(invitation.expires_at).toLocaleString()}{invitation.max_uses ? ` · ${invitation.max_uses} uses maximum` : " · Unlimited uses"}</p>
            <button className="auth-back" type="button" disabled={busy} onClick={() => void revokeInvitation()}>Revoke invitation</button>
          </div>
        )}
        <details className="community-create-panel" open={communities.length === 0}>
          <summary>Create a Community</summary>
          <form className="community-create" onSubmit={createCommunity}>
            <div className="field"><label htmlFor="community-name">Name</label><input id="community-name" required minLength={2} maxLength={80} value={name} onChange={(event) => setName(event.target.value)} placeholder="Builders Guild" /></div>
            <div className="field"><label htmlFor="community-description">Description <span>(optional)</span></label><textarea id="community-description" maxLength={500} value={description} onChange={(event) => setDescription(event.target.value)} placeholder="What brings this Community together?" /></div>
            <button className="auth-submit" type="submit" disabled={busy || ownedCommunityCount >= 3}>{busy ? "Working…" : ownedCommunityCount >= 3 ? "Three-Community limit reached" : "Create Community"}</button>
          </form>
        </details>
        {error && <p className="form-error" role="alert">{error}</p>}
        {notice && <p className="form-notice" role="status">{notice}</p>}
      </footer>
    </section>
  );
}
