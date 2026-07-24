# Runtime SQLite State Plane

This package is the production persistence implementation for the Runtime
Binding, Evidence and Governance Owners on one machine. It stores Binding
Facts/Sets/Grants, Review Binding projections, EvidenceSubject
history/current/mutation commits, Review Evidence applicability
history/current/publish receipts, and Review Decision Policy/Authority/Scope
source facts plus exact-current projections in SQLite transactions. Operation
Review Authorization V4/V5 facts use append-only history, a separate current
index with a highest-revision ABA watermark, and one shared active
`(Tenant, Operation, Effect)` guard in the same transaction. V5 also exposes
historical exact Inspect for lost-reply recovery. Human Multi-Sign V2 quorum
Policy uses the same append-only/current/full-ref-CAS discipline, with stable
tenant/domain identity and immutable K/N, role, veto, delegation, self-review,
duration and TTL projections. WAL, strict JSON row seals,
schema digest verification and `PRAGMA integrity_check` are enforced.

The package deliberately provides no HA, remote replication, failover, shared
filesystem, multi-host topology or SLA claim. `RenewBindingSetV2` remains fail
closed because its independent Attestation Owner cannot participate in the same
SQLite snapshot. Evidence current Gateways still perform their public S1/S2,
TTL and clock checks above this atomic store. Review Decision Governance uses
the existing public Gateway for Review-owned Target/Assignment proofs and
source-fact S1/S2; this package does not persist those Review proofs and never
infers a tenant Policy default. No production composition root is created here.
This store persists Authorization facts only; it does not create a Permit,
Begin, Provider execution point, or Review-domain verdict.
