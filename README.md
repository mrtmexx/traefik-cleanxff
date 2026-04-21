# traefik-cleanxff

A [Traefik](https://traefik.io) middleware plugin that removes **trusted proxy
IP addresses** from the `X-Forwarded-For` header before the request is
forwarded to the upstream backend.

## Why

When Traefik sits behind one or more trusted proxies (cloud load balancer,
CDN, ingress controller), those proxies' IPs end up in the `X-Forwarded-For`
chain. Traefik itself uses `forwardedHeaders.trustedIPs` on the entryPoint to
correctly identify the real client IP, but it does **not** strip the trusted
hops from the header that is forwarded to the backend. This plugin does
exactly that.

**Before** (what the backend sees without the plugin):

```
X-Forwarded-For: 1.1.1.1, 173.245.48.5, 10.0.0.7
```

**After**:

```
X-Forwarded-For: 1.1.1.1
```

Any untrusted intermediate hops are preserved; only IPs that fall within the
configured trusted CIDRs are removed.

## Compatibility

- Traefik **v2** and **v3**.
- Go **1.21+** (for the Yaegi runtime bundled with Traefik).
- No external dependencies. Pure standard library.

## Installation

### Static configuration (YAML)

```yaml
experimental:
  plugins:
    cleanxff:
      moduleName: github.com/mrtmexx/traefik-cleanxff
      version: v0.1.0
```

### Static configuration (CLI flags)

```
--experimental.plugins.cleanxff.modulename=github.com/mrtmexx/traefik-cleanxff
--experimental.plugins.cleanxff.version=v0.1.0
```

### Local development

Drop the repository into `/plugins-local/src/github.com/mrtmexx/traefik-cleanxff/`
inside the Traefik container and configure:

```yaml
experimental:
  localPlugins:
    cleanxff:
      moduleName: github.com/mrtmexx/traefik-cleanxff
```

## Usage

### Kubernetes CRD (Traefik v3)

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: clean-xff
  namespace: prod
spec:
  plugin:
    cleanxff:
      trustedCIDRs:
        - "10.0.0.0/8"
        - "192.168.0.0/16"
        - "173.245.48.0/20"
```

Attach it to an IngressRoute:

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: my-app
  namespace: prod
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`example.com`)
      kind: Rule
      middlewares:
        - name: clean-xff
      services:
        - name: my-app
          port: 80
```

### Kubernetes CRD (Traefik v2)

Same as above, but use `traefik.containo.us/v1alpha1` instead of `traefik.io/v1alpha1`.

### File provider

```yaml
http:
  middlewares:
    clean-xff:
      plugin:
        cleanxff:
          trustedCIDRs:
            - "10.0.0.0/8"
            - "173.245.48.0/20"

  routers:
    my-router:
      rule: "Host(`example.com`)"
      middlewares:
        - clean-xff
      service: my-service
```

## Configuration

| Field          | Type       | Default             | Description                                                                 |
|----------------|------------|---------------------|-----------------------------------------------------------------------------|
| `trustedCIDRs` | `[]string` | **required**        | List of CIDR ranges whose IPs will be stripped from the header. IPv4 & IPv6.|
| `headerName`   | `string`   | `X-Forwarded-For`   | Name of the header to clean. Override only for non-standard setups.         |

## Recommended setup

Use the plugin **together with** Traefik's built-in `forwardedHeaders.trustedIPs`
on the entryPoint. The built-in mechanism protects against clients submitting
forged XFF headers; the plugin strips the trusted IPs from the chain before
it reaches the backend.

```yaml
entryPoints:
  websecure:
    address: ":443"
    forwardedHeaders:
      trustedIPs:
        - "10.0.0.0/8"
        - "173.245.48.0/20"
```

Keep the CIDR lists in sync between `forwardedHeaders.trustedIPs` and the
plugin's `trustedCIDRs`.

## How it works

1. The plugin reads every occurrence of the configured header (XFF can appear
   as multiple header lines, each with a comma-separated list).
2. It tokenises, trims, and parses each element as an IP.
3. Each IP is checked against the trusted CIDR list; trusted IPs are dropped,
   untrusted IPs are kept in their original order.
4. The header is rewritten (or removed entirely if nothing remains).
5. Non-IP tokens (which shouldn't appear in XFF but sometimes do) are
   preserved unchanged as a safety net.

## Testing

```
go test ./...
```

## License

MIT
