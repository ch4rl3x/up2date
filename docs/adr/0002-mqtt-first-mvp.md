# ADR 0002: MQTT-First MVP Without Backend

- Status: Accepted
- Date: 2026-03-24

## Context

The riskiest early questions for `up2date` are not UI or persistence questions. They are:

- can a sidecar reliably inspect nearby Docker containers
- what should the normalized state payload look like
- how often should the system publish state
- what does a later backend actually need to consume

Building a backend before those questions are settled would likely create throwaway ingestion logic.

## Decision

The MVP will start without a dedicated backend.

Instead:

- one `up2date-agent` sidecar runs per Docker host
- it reads local container state from the Docker socket
- it publishes a full node snapshot to MQTT every minute
- humans inspect the result in MQTT Explorer during the first validation phase

The first backend must consume the same MQTT snapshot contract rather than replacing it.

## Consequences

Positive:

- smallest possible validation loop
- easy to debug from a laptop
- transport contract becomes concrete early
- backend design can emerge from observed data instead of guesses

Negative:

- no history
- no durable read model yet
- stale detection is manual until a backend exists
- retained messages or frequent republishing are needed for a good observer experience

## Rejected Alternatives

### One-Time Push At Container Start

Rejected because it is too fragile for observer restarts and misses later state changes.

### Backend-First HTTP Ingestion

Rejected for now because it would force an ingestion model before the snapshot contract is stable.

### MQTT Per Container As Primary Model

Rejected for the MVP because a single full-node snapshot is easier to debug and easier for a later backend to consume consistently.
