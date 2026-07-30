package caddy_test

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// Example shows the two objects Dinchy pushes for the panel plus one deployment: one addressable
// route nesting every entrypoint, and one certificate automation policy naming their hosts.
func Example() {
	cfg := config.DefaultCaddy()
	cfg.PanelHost = "panel.example.com"
	cfg.ACMEEmail = "ops@example.com"
	cfg.HSTSMaxAge = 0

	contribution, err := caddy.BuildContribution(cfg, []caddy.Route{
		{Owner: "deployments", Host: "whoami.example.com", Upstream: "127.0.0.1:32768"},
		{Owner: caddy.PanelOwner, Host: "panel.example.com", Upstream: "127.0.0.1:8080"},
	})
	if err != nil {
		fmt.Println("build failed:", err)
		return
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(contribution.Policy); err != nil {
		fmt.Println("encode failed:", err)
		return
	}
	fmt.Println(contribution.Route.ID, contribution.Route.Match[0].Host, contribution.RouteCount)
	// Output:
	// {
	//   "@id": "dinchy.dinchy.tls",
	//   "subjects": [
	//     "panel.example.com",
	//     "whoami.example.com"
	//   ],
	//   "issuers": [
	//     {
	//       "module": "acme",
	//       "email": "ops@example.com"
	//     }
	//   ]
	// }
	// dinchy.dinchy.routes [panel.example.com whoami.example.com] 2
}

// ExampleRoute_Resolve shows the canonicalization every Route goes through before it is
// compared or rendered, so two spellings of one host cannot both claim it.
func ExampleRoute_Resolve() {
	route := caddy.Route{Owner: "deployments", Host: "  App.Example.COM  ", Upstream: "127.0.0.1:32768"}

	fmt.Println(route.Resolve().Host)
	// Output: app.example.com
}
