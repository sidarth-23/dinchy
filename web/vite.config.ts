import tailwindcss from "@tailwindcss/vite"
import { devtools } from "@tanstack/devtools-vite"

import { tanstackRouter } from "@tanstack/router-plugin/vite"

import viteReact from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const config = defineConfig({
	resolve: { tsconfigPaths: true },
	// Neither the browser nor the host reaches this server directly: it runs in a container on
	// the edge's network, and the edge terminates TLS on https://localhost:8443 and forwards
	// every non-/api request here. That is what keeps the app and the API same-origin.
	// DINCHY_FRONTEND_URL must name this service and the port below.
	server: {
		port: 3000,
		// The edge's upstream is a fixed port, so falling back to 3001 would surface as a blank
		// page behind the proxy instead of an error here.
		strictPort: true,
		// Bind every interface: the only client is another container, so listening on loopback
		// alone would leave the edge unable to reach it at all.
		host: true,
		// Vite rejects a request whose Host header it does not recognise, and the edge forwards
		// the browser's Host verbatim — which is what the CORS origin check depends on.
		allowedHosts: ["localhost", ".localhost"],
		// HMR is a WebSocket the browser opens against the page's origin, so it has to dial
		// the edge rather than this port.
		hmr: { protocol: "wss", clientPort: 8443 },
	},
	plugins: [
		devtools(),
		tailwindcss(),
		tanstackRouter({ target: "react", autoCodeSplitting: true }),
		viteReact(),
	],
})

export default config
