package internal

import (
	"context"
	"net/http"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

// connectFunc handles a single subresource request. ns/name are resolved from
// the request path; it writes the response directly to w.
type connectFunc func(ctx context.Context, ns, name string, w http.ResponseWriter, req *http.Request)

// connecter is a generic connector subresource (like pods/log, pods/exec):
// genericapiserver hands us the raw http.Handler, so we have full control over
// status, Content-Type and streaming.
type connecter struct {
	methods []string
	fn      connectFunc
}

func newConnecter(methods []string, fn connectFunc) *connecter {
	return &connecter{methods: methods, fn: fn}
}

var (
	_ rest.Storage   = &connecter{}
	_ rest.Connecter = &connecter{}
	_ rest.Scoper    = &connecter{}
)

func (c *connecter) New() runtime.Object      { return &yassv1.Experiment{} }
func (c *connecter) Destroy()                 {}
func (c *connecter) NamespaceScoped() bool    { return true }
func (c *connecter) ConnectMethods() []string { return c.methods }
func (c *connecter) NewConnectOptions() (runtime.Object, bool, string) {
	return nil, false, ""
}

func (c *connecter) Connect(ctx context.Context, id string, _ runtime.Object, _ rest.Responder) (http.Handler, error) {
	ns, _ := request.NamespaceFrom(ctx)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c.fn(ctx, ns, id, w, req)
	}), nil
}
