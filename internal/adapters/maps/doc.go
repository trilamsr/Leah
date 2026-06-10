// Package maps is Leah's adapter for Google Maps Platform. It exposes a narrow
// Adapter surface (Geocode, ReverseGeocode, Route, POINearby) and routes every
// RPC through the operator-attestation gate before issuing the HTTP request.
// W56 lands a skeleton + three RPCs running against an injectable HTTPClient
// (httptest in suite, real net/http in production); SDK adoption and the
// POIAlongRoute corridor algorithm are deferred to W57 and the wiring wave.
package maps
