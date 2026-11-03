package api

import (
	"testing"

	mediarequests "github.com/Silo-Server/silo-server/internal/requests"
)

// The HTTP API and the reconcile task each build their own requests service, so
// each must attach the download-service client. A service served by the API
// that missed it closed a request in Silo while leaving the download in Seerr.
func TestAttachRequestTargetsWiresTheDownloadService(t *testing.T) {
	AttachRequestTargets(nil) // a nil service is a no-op, not a panic

	svc := &mediarequests.Service{}
	if svc.HasTargetService() {
		t.Fatal("a bare service already reports a download-service client")
	}
	AttachRequestTargets(svc)
	if !svc.HasTargetService() {
		t.Fatal("AttachRequestTargets did not wire a download-service client")
	}
}
