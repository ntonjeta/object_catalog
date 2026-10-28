# Federation: user guide

Who can see your catalog on the fediverse, what gets announced, and how to
set it up. Decision background: [ADR-0002](../adr/0002-federation.md),
[ADR-0003](../adr/0003-federation-adapter-split.md) and
[ADR-0004](../adr/0004-peer-join-and-mirror.md).

## What federation does here

The whole instance appears as one account — a *Service* actor named
`catalog@your-host`. It behaves like a announcing account, not a chatting
person:

- Other fediverse accounts can **follow** `catalog@your-host`.
- When the administrator creates, edits, marks busy/released, or deletes an
  object, followers receive a signed `Create` / `Update` / `Delete`
  activity. Each object appears as an `Article` with name, description,
  photo, category, tags, and an availability note (`available` or
  `on loan until <time>`).
- Deletes keep a `Tombstone` at the object's ActivityPub URL, so past
  announcements don't dangle.
- Remote accounts follow you over the protocol (`Follow`/`Undo`
  activities), and the admin panel lists them under **Followers**.
- You can also **join** another instance in the other direction: the admin
  panel then follows that peer and mirrors the objects it announces into
  your own catalog. See [Join a peer instance](#join-a-peer-instance).

Not yet federated (planned, see the [federation plan](../activitypub-plan.md)):

- Remote users requesting a reservation. Incoming requests are not accepted
  at this stage; activity types the instance does not act on are acknowledged
  and dropped.
- Likes/boosts/comments on announcements are ignored.

## Setup

1. Set `BASE_URL` to the public origin users reach the site at, including
   the scheme, e.g. `BASE_URL=https://catalog.example.org`. It is required;
   the server refuses to start without it. `localhost` URLs are fine for
   trying it out, but only the instance itself can federate with a
   reachable address.
2. Serve over HTTPS in production and set `COOKIE_SECURE=true`. The
   `http(s)` scheme of `BASE_URL` must match how peers actually reach you
   (behind a reverse proxy, use the public URL, not the internal one).
3. No other configuration is needed. Federation runs whenever the server
   runs; announcements are delivered in the background with automatic
   retries (30 s to 30 min backoff).

Everything peers see is already public in the catalog: object name,
description, photo, category, tags, and availability. Federation adds no
new data disclosure — it pushes the public view to followers.

## Join a peer instance

The admin panel has a **Federation** section, visible after you log in. It
shows your own handle, a join form, the peers you follow, your followers,
and the outbox with delivery state. Use it to link two instances — yours
and a friend's — so each shows the other's objects.

Type the peer's handle or actor URL into the join form and press **Join**:

```
catalog@peer.example
https://peer.example/ap/actors/catalog
```

What happens next:

1. The peer is looked up immediately: a handle is resolved through
   WebFinger, an actor URL is fetched directly. If the peer is unreachable
   or does not exist, the join fails with a message and nothing is stored.
   Joining your own instance is refused.
2. The peer shows up in the list with the state **pending**: the `Follow`
   activity is queued in the outbox and waits for delivery.
3. When the peer accepts the follow, the state flips to **accepted** and
   its announcements start to arrive. Press **Refresh** to see it; the
   handshake usually takes a few seconds.

While a peer is joined, the objects it announces appear in your catalog
list with a `via peer.example` badge, and the object name links to the
peer's own ActivityPub article URL. Those rows are read-only copies: you
cannot edit them, and they are removed from your catalog when you remove
the peer.

**Remove** next to a peer leaves it. An `Undo` is queued for the peer, the
mirrored objects are deleted, and the peer leaves the list. Joining the
same peer again starts over from **pending** and mirrors only what the
peer announces from then on — older objects are not backfilled.

Mirrored content is only accepted from a peer you joined and that accepted
the follow. A remote account that follows you, or that sends a correctly
signed announcement on its own, can never write into your catalog.

## Verify your instance

```bash
# discovery: should return the actor URL
curl -s 'https://catalog.example.org/.well-known/webfinger?resource=acct:catalog@catalog.example.org'

# actor document: type Service, publicKey present
curl -s -H 'Accept: application/activity+json' \
  'https://catalog.example.org/ap/actors/catalog'

# outbox: recent announcements
curl -s -H 'Accept: application/activity+json' \
  'https://catalog.example.org/ap/outbox?limit=5'
```

Then, from any fediverse account (Mastodon and similar), search for
`catalog@catalog.example.org`, follow, and create or edit an object as the
administrator — the announcement should appear in the follower's timeline.

## Troubleshooting

- **Follow fails in the remote app.** `BASE_URL` must be reachable from the
  internet and match the host in the handle exactly
  (`catalog@X` requires `BASE_URL` host `X`). Check both curl commands above
  succeed over the public URL.
- **Follows are accepted but nothing appears.** Announcements only cover
  changes *after* the follow; older posts are not re-broadcast. Edit or
  create an object to test.
- **Server logs show `federation:` lines** (delivery failures, rejected
  inbox signatures). Brief delivery failures self-heal via retries; while
  any follower is unreachable the whole entry keeps backing off and is
  re-sent to every follower once it succeeds, so duplicate announcements
  are possible.
- **Remote server says signature verification failed.** Your instance signs
  with its own key (`/ap/actors/catalog#main-key`); if the remote recently
  changed keys, delivery retries with the fresh key automatically.
- **Deleting the database file resets the instance identity** (signing key).
  Followers from the old key will stop accepting announcements; back up the
  SQLite file like any other identity secret.
- **A peer stays `pending`.** The `Follow` is still queued or the peer never
  answered. Look at the outbox table: the `follow` row shows how many
  attempts were made and when the next retry is due. A peer that was down
  during the join is retried automatically (30 s to 30 min backoff), and a
  restart of your server resumes the pending entries.
- **The join fails right away.** The peer could not be looked up — wrong
  handle, no WebFinger record, or an instance that is not reachable at that
  address. Nothing was stored, so you can try again once the peer is up.
- **A peer's objects do not show up in your catalog.** Only announcements
  sent after the peer was `accepted` are mirrored, and a peer may only
  announce objects published under its own address — an object announced
  with a foreign address is ignored on purpose.
- **A removed peer's objects are gone.** Mirrored copies belong to the
  peer; removing it deletes them from your catalog.

## Limitations to know about

- The inbox accepts `Follow`/`Undo` from any correctly signed account; there
  is no approval queue, block list, or rate limiting yet.
- Announcements carry the object photo URL as stored; keep `UPLOAD_DIR`
  content reachable at `BASE_URL` or federated posts show broken images.
- Objects mirrored from a peer are read-only copies. You cannot edit them,
  they disappear when you remove the peer, and joining again mirrors only
  what is announced afterwards.
- A peer can send any text it likes in names, descriptions and tags. It is
  shown literally — never as markup — but treat it as untrusted content.
