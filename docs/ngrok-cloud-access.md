# Governed ngrok cloud access

HAI is local-first. The `cloud-tunnel` Compose profile is deliberately off by
default and is the only supported route for temporary public HTTPS access to a
loopback-bound local HAI installation.

Before enabling it, create `.env.local` from `.env.example`, use real generated
secrets, reserve an HTTPS ngrok domain, and configure a dedicated ngrok token.
The tunnel refuses to start unless production mode, secure cookies, loopback
gateway binding, and disabled public A2A exposure are all explicit.

Validate the configuration without creating a tunnel:

```powershell
.\scripts\start-ngrok.ps1 -ValidateOnly
```

Start or stop the tunnel only from the checkout that owns the `018-hai`
containers:

```powershell
.\scripts\start-ngrok.ps1
.\scripts\start-ngrok.ps1 -Stop
```

The launcher rejects placeholder secrets, another checkout's containers, an
unreserved URL, insecure cookies, non-loopback gateway binding, and any attempt
to advertise the local-only A2A bridge. Public access does not bypass normal
application authentication or approval rules.
