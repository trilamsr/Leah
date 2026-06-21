// Package contracts holds shared cross-package interface declarations.
//
// Goal: prevent interface drift across adapters. Each adapter (gmail, gcal,
// iMessage, FaceTime, maps, feeds, voice/listener, recommend, work-tools)
// imports contracts.Attestor / TokenSource / OSExec / HTTPClient / Notifier
// rather than re-declaring its own.
//
// This package MUST NOT import any other leah package. It declares
// contracts only — no implementations, no helpers, no constants.
package contracts
