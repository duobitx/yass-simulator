package internal

import (
	"context"
	"fmt"
	"net/http"

	"github.com/m-szalik/goutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/registry/rest"
	genericserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
)

// Options configures the aggregated API server.
type Options struct {
	SecurePort    int
	CertDir       string
	AdvertiseHost string
}

func OptionsFromEnv() *Options {
	return &Options{
		SecurePort:    goutils.Env("SECURE_PORT", 6443),
		CertDir:       goutils.Env("CERT_DIR", ""),
		AdvertiseHost: goutils.Env("ADVERTISE_HOST", "yass-experiment-apiservice.yass-system.svc"),
	}
}

// Run builds and runs the aggregated API server (a real kubernetes APIService
// backend). It is a facade with no etcd: every resource/subresource is served
// by custom REST storage that reads CRs and proxies the per-namespace
// experiment Services.
func Run(ctx context.Context, opts *Options) error {
	backend, err := NewBackend()
	if err != nil {
		return fmt.Errorf("backend: %w", err)
	}

	cfg := genericserver.NewRecommendedConfig(Codecs)

	secure := genericoptions.NewSecureServingOptions().WithLoopback()
	secure.BindPort = opts.SecurePort
	secure.ServerCert.CertDirectory = opts.CertDir
	if err := secure.MaybeDefaultWithSelfSignedCerts(opts.AdvertiseHost, nil, nil); err != nil {
		return fmt.Errorf("self-signed serving cert: %w", err)
	}
	if err := secure.ApplyTo(&cfg.SecureServing, &cfg.LoopbackClientConfig); err != nil {
		return fmt.Errorf("serving: %w", err)
	}

	authn := genericoptions.NewDelegatingAuthenticationOptions()
	if err := authn.ApplyTo(&cfg.Authentication, cfg.SecureServing, cfg.OpenAPIConfig); err != nil {
		return fmt.Errorf("delegated authn: %w", err)
	}
	authz := genericoptions.NewDelegatingAuthorizationOptions()
	if err := authz.ApplyTo(&cfg.Authorization); err != nil {
		return fmt.Errorf("delegated authz: %w", err)
	}

	server, err := cfg.Complete().New("experiment-apiservice", genericserver.NewEmptyDelegate())
	if err != nil {
		return fmt.Errorf("new generic server: %w", err)
	}

	apiGroupInfo := genericserver.NewDefaultAPIGroupInfo(APIGroup, Scheme, metav1.ParameterCodec, Codecs)
	apiGroupInfo.VersionedResourcesStorageMap[APIVersion] = map[string]rest.Storage{
		resource:              newExperimentREST(backend),
		resource + "/time":    newConnecter([]string{http.MethodGet}, backend.handleTime),
		resource + "/events":  newConnecter([]string{http.MethodGet}, backend.handleEvents),
		resource + "/start":   newConnecter([]string{http.MethodPost}, backend.handleStart),
		resource + "/fsnodes": newConnecter([]string{http.MethodGet}, backend.handleFsNodes),
		resource + "/results": newConnecter([]string{http.MethodGet}, backend.handleResults),
	}
	if err := server.InstallAPIGroup(&apiGroupInfo); err != nil {
		return fmt.Errorf("install api group: %w", err)
	}

	return server.PrepareRun().RunWithContext(ctx)
}
