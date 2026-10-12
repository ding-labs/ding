package access

import (
	"net/http"
	"net/url"
)

// Routes is the closed inventory of authenticated operations. Authentication,
// static assets and health checks are handled by their respective adapters.
var Routes = map[string]Action{
	"GET /v1/info":                                 Inspect,
	"GET /v1/status":                               Inspect,
	"GET /v1/watches":                              Inspect,
	"GET /v1/watches/{id}":                         Inspect,
	"GET /v1/watches/{id}/export":                  Inspect,
	"GET /v1/events":                               Inspect,
	"GET /v1/events/{id}":                          Inspect,
	"GET /v1/events/{id}/observations":             Inspect,
	"GET /v1/deliveries/{id}":                      Inspect,
	"GET /v1/reference":                            Inspect,
	"POST /v1/apply":                               Manage,
	"POST /v1/watches/{id}/lifecycle":              Manage,
	"POST /v1/deliveries/{id}/retry":               Retry,
	"POST /v1/onboarding/preview":                  Preview,
	"POST /v1/tools/compile":                       Preview,
	"POST /v1/tools/test":                          Preview,
	"POST /v1/tools/replay":                        Preview,
	"POST /v1/backup":                              Host,
	"GET /v1/doctor":                               Inspect,
	"POST /v1/console/backup":                      Host,
	"GET /v1/console/backup/{id}":                  Host,
	"GET /v1/console/doctor":                       Inspect,
	"POST /v1/browser/handoff":                     Host,
	"POST /v1/local/shutdown":                      Host,
	"GET /v1/local/updates":                        Host,
	"GET /v1/desktop/status":                       Host,
	"POST /v1/desktop/settings":                    Host,
	"POST /v1/desktop/test":                        Host,
	"POST /v1/demo":                                Host,
	"POST /v1/ingest/{id}":                         Ingest,
	"GET /v1/cloud/export":                         Inspect,
	"GET /v1/cloud/usage":                          Inspect,
	"GET /v1/cloud/secrets":                        Credentials,
	"POST /v1/cloud/secrets":                       Credentials,
	"DELETE /v1/cloud/secrets/{name}":              Credentials,
	"GET /v1/cloud/models":                         Inspect,
	"POST /v1/cloud/models":                        Inspect,
	"POST /v1/cloud/destination-test":              Destinations,
	"DELETE /v1/workspace":                         DeleteWorkspace,
	"GET /v1/services":                             Members,
	"POST /v1/services":                            Members,
	"DELETE /v1/services/{id}":                     Members,
	"GET /v1/members":                              Members,
	"POST /v1/members":                             Members,
	"PATCH /v1/members/{id}":                       Members,
	"DELETE /v1/members/{id}":                      Members,
	"GET /v1/audit":                                Inspect,
	"GET /v1/handoffs":                             Inspect,
	"GET /v1/handoffs/{id}":                        Inspect,
	"GET /v1/handoffs/preflight/{watch}":           Inspect,
	"POST /v1/handoffs/prepare":                    Move,
	"POST /v1/handoffs/pause":                      Move,
	"POST /v1/handoffs/{id}/test":                  Move,
	"POST /v1/handoffs/{id}/activate":              Move,
	"POST /v1/handoffs/{id}/finish":                Move,
	"POST /v1/handoffs/{id}/cancel":                Move,
	"POST /v1/handoffs/{id}/release":               Move,
	"POST /v1/integrations/grants":                 Members,
	"GET /v1/integrations/grants":                  Members,
	"DELETE /v1/integrations/grants/{id}":          Members,
	"GET /v1/integrations/capabilities":            Inspect,
	"POST /v1/integrations/preview":                Preview,
	"POST /v1/integrations/apply":                  Manage,
	"POST /v1/integrations/watches/{id}/lifecycle": Manage,
	"POST /v1/integrations/deliveries/{id}/retry":  Retry,
	"GET /v1/integrations/operations/{key}":        Inspect,
}

var routeMux = func() *http.ServeMux {
	Routes["POST /v1/integrations/transfers/step"] = Move
	for _, path := range []string{"/v1/integrations/transfers", "/v1/integrations/transfers/{id}", "/v1/integrations/transfers/preflight/{watch}"} {
		Routes["GET "+path] = Inspect
	}
	for _, kind := range []string{"watches", "events", "deliveries", "destinations"} {
		Routes["GET /v1/console/"+kind] = Inspect
		Routes["GET /v1/integrations/"+kind] = Inspect
	}
	for _, kind := range []string{"watches", "events", "deliveries"} {
		Routes["GET /v1/integrations/"+kind+"/{id}"] = Inspect
	}
	mux := http.NewServeMux()
	for pattern := range Routes {
		mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	}
	return mux
}()

func Route(method, path string) (Action, bool) {
	_, pattern := routeMux.Handler(&http.Request{Method: method, URL: &url.URL{Path: path}})
	a, ok := Routes[pattern]
	return a, ok
}
