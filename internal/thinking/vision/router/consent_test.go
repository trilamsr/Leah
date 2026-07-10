package router

import "testing"

func TestConsent_DefaultDenied(t *testing.T) {
	s := newMemConsent()
	if s.Granted("live_screen") {
		t.Fatal("default must be denied")
	}
}

func TestConsent_GrantSessionRegisters(t *testing.T) {
	s := newMemConsent()
	s.Grant("live_screen", ScopeThisSession)
	if !s.Granted("live_screen") {
		t.Fatal("session grant must register")
	}
}

func TestConsent_RevokeClears(t *testing.T) {
	s := newMemConsent()
	s.Grant("screenshot", ScopePersistent)
	s.Revoke("screenshot")
	if s.Granted("screenshot") {
		t.Fatal("revoke must clear grant")
	}
}

func TestConsent_GrantOverwrites(t *testing.T) {
	s := newMemConsent()
	s.Grant("screenshot", ScopeThisSession)
	s.Grant("screenshot", ScopePersistent)
	if !s.Granted("screenshot") {
		t.Fatal("regrant must keep mode granted")
	}
}
