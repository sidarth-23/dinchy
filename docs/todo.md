- potentially adding todo md in the root with checkboxes for features

- make sure organisation and other things works properly with totp and sso also. Check the password based flow also for auth
- We need not have separate plugins txt. For this, we can use caddyserver from source, build it and have the extensions from there. From there on, we can rely on xcaddy to view the plugins, the ones we support with ui and which we don't
- Have separate files to handle main app and caddy. Currently app.go is fusing both
