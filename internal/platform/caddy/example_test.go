package caddy_test

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// Example shows the configuration Dinchy pushes for the panel plus one deployment.
func Example() {
	cfg := config.DefaultCaddy()
	cfg.PanelHost = "panel.example.com"
	cfg.ACMEEmail = "ops@example.com"
	cfg.HSTSMaxAge = 0
	cfg.StoragePath = ""

	built, err := caddy.BuildConfig(cfg, []caddy.Route{
		{Owner: "deployments", Host: "whoami.example.com", Upstream: "127.0.0.1:32768"},
		{Owner: caddy.PanelOwner, Host: "panel.example.com", Upstream: "127.0.0.1:8080"},
	})
	if err != nil {
		fmt.Println("build failed:", err)
		return
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(built.Apps.HTTP.Servers[caddy.ServerName].Routes); err != nil {
		fmt.Println("encode failed:", err)
	}
	// Output:
	// [
	//   {
	//     "match": [
	//       {
	//         "host": [
	//           "panel.example.com"
	//         ]
	//       }
	//     ],
	//     "handle": [
	//       {
	//         "handler": "reverse_proxy",
	//         "upstreams": [
	//           {
	//             "dial": "127.0.0.1:8080"
	//           }
	//         ],
	//         "headers": {
	//           "request": {
	//             "set": {
	//               "X-Forwarded-For": [
	//                 "{http.request.remote.host}"
	//               ],
	//               "X-Forwarded-Host": [
	//                 "{http.request.host}"
	//               ],
	//               "X-Forwarded-Proto": [
	//                 "{http.request.scheme}"
	//               ]
	//             },
	//             "delete": [
	//               "X-Real-IP",
	//               "True-Client-IP",
	//               "Forwarded"
	//             ]
	//           }
	//         }
	//       }
	//     ],
	//     "terminal": true
	//   },
	//   {
	//     "match": [
	//       {
	//         "host": [
	//           "whoami.example.com"
	//         ]
	//       }
	//     ],
	//     "handle": [
	//       {
	//         "handler": "reverse_proxy",
	//         "upstreams": [
	//           {
	//             "dial": "127.0.0.1:32768"
	//           }
	//         ],
	//         "headers": {
	//           "request": {
	//             "set": {
	//               "X-Forwarded-For": [
	//                 "{http.request.remote.host}"
	//               ],
	//               "X-Forwarded-Host": [
	//                 "{http.request.host}"
	//               ],
	//               "X-Forwarded-Proto": [
	//                 "{http.request.scheme}"
	//               ]
	//             },
	//             "delete": [
	//               "X-Real-IP",
	//               "True-Client-IP",
	//               "Forwarded"
	//             ]
	//           }
	//         }
	//       }
	//     ],
	//     "terminal": true
	//   }
	// ]
}

// ExampleRoute_Resolve shows the canonicalization every Route goes through before it is
// compared or rendered, so two spellings of one host cannot both claim it.
func ExampleRoute_Resolve() {
	route := caddy.Route{Owner: "deployments", Host: "  App.Example.COM  ", Upstream: "127.0.0.1:32768"}

	fmt.Println(route.Resolve().Host)
	// Output: app.example.com
}
