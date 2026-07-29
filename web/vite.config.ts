import tailwindcss from "@tailwindcss/vite"
import { devtools } from "@tanstack/devtools-vite"

import { tanstackRouter } from "@tanstack/router-plugin/vite"

import viteReact from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const config = defineConfig({
	resolve: { tsconfigPaths: true },
	// The browser never reaches this server directly: Caddy terminates TLS on
	// https://localhost:8443 and forwards every non-/api request here, which is what keeps
	// the app and the API same-origin. DINCHY_DEV_PROXY_URL must name the port below.
	server: {
		port: 3000,
		// Caddy's upstream is a fixed port, so falling back to 3001 would surface as a blank
		// page behind the proxy instead of an error here.
		strictPort: true,
		// HMR is a WebSocket the browser opens against the page's origin, so it has to dial
		// Caddy rather than this port.
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
