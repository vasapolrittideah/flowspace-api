# ADR-0007: Push changes to clients over Server-Sent Events

- **Status:** Accepted
- **Date:** 2026-09-05
- **Deciders:** Vasapol Rittideah
- **Tags:** `infra`, `domain`

## Context

FlowSpace is a shared workspace. When someone moves a card, everyone looking at
that board should see it move without pressing refresh. Comments, presence and
notification badges have the same requirement.

ADR-0006 turned this from a nicety into a necessity. A workflow that crosses
contexts returns an acknowledgement and a saga identifier, not a finished
result, so the interface has no way to know the work completed unless the
server tells it. Without a push channel every such workflow needs a polling
loop, and the product becomes a collection of spinners that lie.

Two facts about the environment constrain the answer. Traffic reaches the
cluster through Cloudflare rather than a public address of our own, so whatever
is chosen has to survive an intermediary proxy that will close an idle
connection. And the whole system runs on one node, so every open connection is
a goroutine, a buffer and a file descriptor drawn from a fixed budget.

There is already a stream of everything that changes: the domain events from
ADR-0004. Realtime should be a projection of that stream to connected clients,
not a second notification path built beside it.

## Decision

Clients receive changes over **Server-Sent Events**, served by the BFF on an
authenticated `GET` that stays open.

```text
event: change
id: 018f4e6b-1d02-7a44-b91c-0f5e8d7a6b22
data: {"resource":"work_item","id":"wi_018f4e6b","project":"pj_018f4e6b"}
```

Each BFF replica joins the broker as **its own consumer group**, named after
the pod, reading from the latest offset. It receives every event and forwards
only those matching a channel one of its connected clients subscribed to. There
is no inbox and no offset bookkeeping, because none of this is required to be
reliable.

### The stream is a hint, never a source of truth

An event says *what changed*, never *what it changed to*. The client decides
whether it cares and refetches through the normal API.

This is the load-bearing rule of the ADR, and it buys three things at once:

- **Reconnection is trivial.** A client that missed messages does not replay
  them; it refetches and is correct again. There is no per-client replay
  buffer, no `Last-Event-ID` cursor, and no way for a missed message to leave
  the interface silently wrong.
- **Authorisation stays on one path.** The stream never carries content, so it
  never needs a second permission check beside the one the fetch already does.
  A subscription is authorised at connect time; anything the client then reads
  is authorised again at read time, and a permission revoked mid-stream fails
  closed at the fetch.
- **Payloads stay small**, which matters when every replica fans out every
  event.

The cost is chattiness: forty cards changing produces forty hints. Clients
coalesce by resource with a short debounce and refetch the collection once,
rather than issuing a request per hint.

### Operating rules

- A heartbeat comment every 15 seconds keeps the intermediary from closing an
  idle stream. Responses set `Cache-Control: no-store` and disable proxy
  buffering.
- Per-client send buffers are **bounded**. A client too slow to drain its
  buffer is disconnected rather than buffered, and reconnects into a refetch.
  Unbounded buffering would turn one stuck laptop into a memory leak.
- Streams are capped per user and per workspace, and idle streams expire.
- Reconnection backoff is jittered. A deploy drops every stream at once, and
  synchronised reconnects would arrive as a self-inflicted thundering herd.
- `sse_active_streams` and `sse_clients_dropped_slow` are the two observables.

### Client to server stays ordinary HTTP

The channel is one-directional by choice. Everything a client sends is a normal
request, so authentication, rate limiting, idempotency keys and tracing all
continue to work exactly as they do everywhere else, with no second
implementation inside a socket.

### Out of scope

This decision does not cover collaborative editing of a text document. Shared
cursors and concurrent character-level editing need a bidirectional low-latency
transport and a convergent data type, and that is a separate decision that
would sit beside this one rather than replace it.

## Consequences

### Positive

- No new protocol and no new infrastructure: it is an HTTP response that does
  not end, and `EventSource` reconnects on its own.
- Degradation is invisible. The worst case of a total realtime failure is an
  interface that updates when the user acts, which is where it started.
- Nothing in the stream needs its own authorisation model.
- Works through Cloudflare and any conforming proxy without special handling
  beyond the heartbeat.

### Negative / accepted costs

- One long-lived connection per open tab, held for as long as the tab is open,
  which is why the per-user cap is part of the decision rather than tuning.
- HTTP/2 is effectively required; under HTTP/1.1 a stream consumes one of the
  six connections a browser allows per origin. The edge provides HTTP/2, so
  this is a constraint on the edge, not a free choice.
- Every replica consumes every event. At two replicas this is cheaper than
  routing; it stops being true as replicas multiply.
- Hint-then-refetch produces more small requests than pushing data would.
- There is no delivery guarantee at all. That is the design, and it only works
  because the client is never allowed to treat the stream as authoritative.

### Neutral

- Because the stream carries no state, replacing the transport later changes
  one adapter on each side and no domain code.

## Alternatives considered

| Option                    | Why not                                                                                                                                                                                                      |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| WebSocket                 | Bidirectional capability that is not needed, bought by moving authentication, rate limiting, tracing and idempotency off the HTTP path and into a bespoke frame protocol that has to reimplement all of them |
| Long polling              | The same connection cost as a stream, with worse latency and far more requests                                                                                                                               |
| Client polling on a timer | Staleness on a shared board is the product, and idle polling from every open tab costs more requests than one idle stream costs connections                                                                  |
| A hosted realtime service | Recurring cost, and every tenant's change stream would pass through a third party                                                                                                                            |
| WebSocket with a CRDT now | Solves collaborative text editing, which is not a requirement yet, and charges every screen that will never need it                                                                                          |

## Revisit when

- Collaborative document editing becomes a requirement. That is a new decision
  about a new transport, not an amendment to this one.
- Replica count or event volume makes full fan-out per replica wasteful. The
  step after this is routing by channel so a replica receives only what its
  clients subscribed to.
- Concurrent connections approach the node's memory or descriptor budget, or
  the refetch traffic triggered by hints outweighs what pushing state would
  have cost.

## References

- [HTML Living Standard — Server-sent events](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- [MDN — EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource)
- [MDN — Using server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)
