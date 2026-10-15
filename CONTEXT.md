# Community Communications

This context describes a low-friction communications platform in which people join communities, create rooms, coordinate through roles, exchange messages, and meet through realtime audio or video.

## Identity and tenancy

**User**:
A person with a platform identity, anchored by a verified email address.
_Avoid_: Account, attendee

**Community**:
The top-level collaboration space that owns members, roles, rooms, and moderation policy. A conference is one kind of Community.
_Avoid_: Server, workspace, event

**Member**:
A User's participation in one Community, including their community-specific profile, roles, and status.
_Avoid_: Attendee, participant

**Username**:
A Member's human-readable identity within a Community. It is unique within that Community, not across the platform.
_Avoid_: Handle, display name

## Conversation

**Room**:
A member-created or organizer-created place for communication inside a Community. Its kind determines whether it supports persistent text, live voice/video, or a moderated stage.
_Avoid_: Chatroom, channel

**Message**:
A durable contribution posted by a Member to a text-capable Room.
_Avoid_: Chat, post

**Thread**:
A focused conversation attached to a root Message within the same Room.
_Avoid_: Sub-room, reply chain

**Reaction**:
A Member's lightweight, emoji-based response to a Message.

## Access and governance

**Role**:
A Community-owned collection of permissions that can be assigned to Members and selectively overridden in Rooms.
_Avoid_: Group, badge

**Permission**:
A named capability, such as viewing a Room, posting Messages, managing Roles, or publishing media.
_Avoid_: Access level, privilege

**Room Override**:
An explicit allow or deny applied to a Role or Member for one Room.
_Avoid_: Room permission

**Moderation Action**:
An auditable intervention such as deleting a Message, muting a Member, removing a Member, or banning a User from a Community.

## Realtime communication

**Presence**:
Ephemeral knowledge that a Member is connected or active. Presence is not a durable attendance record.

**Call**:
A live audio/video session associated with a voice-capable Room.
_Avoid_: Meeting, RTC session

**Stage**:
A moderated Call in which a small set of Members may publish media while a larger audience subscribes.

**Speaker**:
A Member temporarily permitted to publish audio, video, or screen media in a Stage.
_Avoid_: Publisher
