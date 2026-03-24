# ADR 0001: Language and Runtime Direction

- Status: Accepted
- Date: 2026-03-23

## Context

`up2date` starts as a CLI or agent-first tool and later grows a web interface.

The system will need to:

- run on many Linux hosts
- fit naturally into containers and sidecars
- interact with Docker, registries, SSH, HTTP APIs, and eventually Proxmox
- ship a central server and a lightweight agent
- keep the core model stable while the UI evolves

## Options Considered

### Kotlin Multiplatform

Pros:

- modern language
- possible code sharing across targets
- strong developer experience for teams already deep in Kotlin

Cons:

- library support for infra-heavy use cases is much stronger on JVM than in commonMain
- Docker, SSH, registry, YAML, and Proxmox integrations would likely become target-specific quickly
- KMP complexity arrives before the domain model is proven
- UI code sharing brings limited benefit for this product compared with the extra build complexity

Verdict:

- technically possible
- not recommended for the MVP

### Kotlin/JVM

Pros:

- excellent language and ecosystem
- strong server-side tooling
- easy future use with Ktor and Kotlin serialization
- good fit if the team already moves fastest in Kotlin

Cons:

- host-side deployment is heavier than a static binary
- agent distribution is less frictionless than Go
- native-image paths add extra complexity if JVM-free delivery becomes important

Verdict:

- viable alternative if team preference outweighs ops simplicity

### Go

Pros:

- simple deployment with static binaries
- first-class fit for CLI, daemon, agent, and container use cases
- strong operational ergonomics
- easy cross-compilation
- natural fit for networking and infrastructure integrations

Cons:

- weaker language ergonomics for complex domain abstractions than Kotlin
- likely separate frontend stack later

Verdict:

- best default fit for the product shape

### TypeScript Full Stack

Pros:

- one language across backend and frontend
- fast iteration for web-facing features

Cons:

- runtime requirements are less ideal for host-side agents
- infra-oriented integrations are workable but less natural than in Go
- packaging and deployment for small agents is more awkward

Verdict:

- not preferred for the core runtime

## Decision

Use Go for the first real implementation of the agent and backend.

Use a separate TypeScript + React web UI when the product needs a richer interface.

Keep contracts, configuration, and domain terminology language-neutral so a later Kotlin or other client remains possible.

## Consequences

Positive:

- low-friction agent deployment
- clean path to single-binary delivery
- strong fit for Docker and infrastructure workflows
- reduced complexity during the first MVP

Negative:

- frontend will not share language with backend
- some domain-heavy code may feel less expressive than Kotlin

## Follow-Up Guidance

If the team later decides to standardize on Kotlin, prefer Kotlin/JVM over Kotlin Multiplatform until:

- the product model is stable
- required libraries are proven across targets
- there is clear value in shared code beyond "one language everywhere"
