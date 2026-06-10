// Package mirror is the 60s daemon-tick sync loop that fans Sync calls
// across every registered MacApp, per spec §4 W26. The mirror itself
// holds no Apple-data attestation — each MacApp.Sync attests in its own
// scope; the loop is a scheduler + per-app status surface.
package mirror
