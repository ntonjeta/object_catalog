# Federation: user guide

Who can see your catalog on the fediverse, what gets announced, and how to
set it up. Decision background: [ADR-0002](../adr/0002-federation.md) and
[ADR-0003](../adr/0003-federation-adapter-split.md).

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
- Followers are managed over the protocol only (`Follow`/`Undo`
  activities). There is no follower list in the admin UI.

Not yet federated (planned, see the [federation plan](../activitypub-plan.md)):

- Remote users requesting a reservation. Incoming requests are not accepted
  at this stage; other activity types are acknowledged and dropped.
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

## Limitations to know about

- The inbox accepts `Follow`/`Undo` from any correctly signed account; there
  is no approval queue, block list, or rate limiting yet.
- Announcements carry the object photo URL as stored; keep `UPLOAD_DIR`
  content reachable at `BASE_URL` or federated posts show broken images.
