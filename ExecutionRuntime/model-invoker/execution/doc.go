// Package execution owns the provider-neutral execution state machine for the
// Praxis semantic union. Adapters can report native candidates, but only this
// package commits globally sequenced events, observed Effects, verification
// records, and the single unified terminal event.
package execution
