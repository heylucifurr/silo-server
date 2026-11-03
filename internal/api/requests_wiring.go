package api

import (
	"context"
	"errors"

	"github.com/Silo-Server/silo-server/internal/plugins"
	mediarequests "github.com/Silo-Server/silo-server/internal/requests"
)

// PluginRequestRouterAdapter adapts plugins.Service to
// mediarequests.RouterClientResolver. The concrete *pluginhost.RequestRouterClient
// satisfies mediarequests.RouterClient, so the adapter names the interface as its
// return type (Go has no return-type covariance).
type PluginRequestRouterAdapter struct {
	Svc *plugins.Service
}

func (a PluginRequestRouterAdapter) RequestRouterClient(ctx context.Context, installationID int, capabilityID string) (mediarequests.RouterClient, error) {
	if a.Svc == nil {
		return nil, errors.New("request router plugin service is not configured")
	}
	return a.Svc.RequestRouterClient(ctx, installationID, capabilityID)
}

// RouterFeatures reads the optional request_router.v1 features the
// capability's stored manifest declares, without launching the plugin.
func (a PluginRequestRouterAdapter) RouterFeatures(ctx context.Context, installationID int, capabilityID string) (mediarequests.RouterFeatures, error) {
	if a.Svc == nil {
		return mediarequests.RouterFeatures{}, errors.New("request router plugin service is not configured")
	}
	descriptor, err := a.Svc.RequestRouterDescriptor(ctx, installationID, capabilityID)
	if err != nil {
		return mediarequests.RouterFeatures{}, err
	}
	return mediarequests.RouterFeatures{
		SupportsSeasons:         descriptor.GetSupportsSeasons(),
		ReportsDownloadProgress: descriptor.GetReportsDownloadProgress(),
		APIKeyOptional:          descriptor.GetApiKeyOptional(),
		BaseURLOptional:         descriptor.GetBaseUrlOptional(),
	}, nil
}

// AttachRequestRouter wires the plugin-backed router provider onto a requests
// service. Both the HTTP handler and the reconcile task call this so the wiring
// lives in one place. With either dependency absent (e.g. a build without the
// plugin service) it leaves the requests service router-less, so fulfillment
// degrades to the existing "no backend configured" failure instead of panicking.
func AttachRequestRouter(svc *mediarequests.Service, pluginService *plugins.Service) {
	if svc == nil || pluginService == nil {
		return
	}
	svc.SetRouterProvider(mediarequests.NewPluginRouterProvider(PluginRequestRouterAdapter{pluginService}))
}

// AttachRequestTargets wires the download-service client (Seerr's own REST API)
// onto a requests service. The admin delete uses it to remove a request from
// Seerr, and the reconcile pass uses it to notice a request Seerr no longer has.
// The HTTP handler and the reconcile task each build their own requests
// service, so both call this: a service served by the API that missed it would
// close a request in Silo while leaving the download in Seerr.
func AttachRequestTargets(svc *mediarequests.Service) {
	if svc == nil {
		return
	}
	svc.SetTargetService(mediarequests.NewSeerrClient(nil))
}
