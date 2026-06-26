# Architecture

How `chart-inspector` is organized, how it boots, and the central idea that makes it work: **resources are discovered by observing the API traffic of a server-side dry-run**, not by parsing rendered manifests.

## A thin service over the shared Helm engine

`chart-inspector` is a single, small HTTP service. The heavy lifting — downloading charts (from a Helm repo, OCI, or `.tgz`), caching them, rendering, and running the server-side dry-run — lives in the shared Krateo Helm library. This service is a thin layer around it: parse a request, run the chart through a dry-run with a request-scoped HTTP tracer attached, and return what the tracer saw.

## The main parts

- **The `/resources` handler** — the core. It fetches the target `Composition` and its `CompositionDefinition`, builds the chart's values, installs a tracer, runs the dry-run, and returns the captured resources.
- **The tracer** — a small HTTP interceptor attached to the dry-run's connection to the API server. It records every API resource the dry-run touches.
- **Small lookup helpers** — fetch the `Composition`, the `CompositionDefinition`, and (when the chart needs credentials) a `Secret`.
- **Health probes** — liveness and readiness endpoints.

## The central idea: observe, don't parse

The handler copies the cluster connection and attaches a per-request tracer to it. Every call Helm makes to the API server during the dry-run — looking objects up, validating CRDs, discovering capabilities — flows through that tracer, which turns each request into a resource entry `{group, version, resource, namespace, name}`. The service never parses the rendered manifest; it watches the traffic. That design choice has direct consequences (under-reporting objects that are never looked up, reporting read-only dependencies, and producing duplicates) — all covered in [`02-api-and-request-lifecycle.md`](./02-api-and-request-lifecycle.md).

```mermaid
flowchart TB
    CDC[CDC caller] -->|asks for the resource list| H[/resources handler]
    H -->|fetch Composition + CompositionDefinition| K8s[(Kubernetes API)]
    H -->|server-side dry-run| HELM[shared Helm engine]
    HELM -->|lookup / validate / discover| K8s
    H -.->|attaches a request-scoped tracer to the dry-run| TR[tracer]
    HELM -.->|every API call flows through| TR
    TR -->|captured resources| H
    H -->|JSON list| CDC
```

## How it boots

The startup sequence is short:

1. Read configuration (debug flag, port, kubeconfig).
2. Build a structured JSON logger (for the logs-ingester).
3. Connect to the cluster — in-cluster by default, or from a kubeconfig — with client-side throttling disabled so the API server's own fairness controls govern load.
4. Build the **long-lived Helm client** once, with a chart cache and a CRD watch that persist across requests. This shared, stateful client is the main reason chart-inspector is a long-running service rather than a library.
5. Register the routes and start the HTTP server, with a generous write timeout to accommodate slow chart downloads and dry-runs.

On shutdown it stops serving, closes the Helm client (stopping the cache cleanup and the CRD watch), and drains in-flight requests.

## Conventions

- Dependencies are assembled once at startup and passed into the handlers (simple dependency injection).
- Errors are returned through the shared HTTP-response helpers, not hand-rolled.
- The real Helm machinery is reused from the shared library; this service deliberately keeps only a minimal tracer of its own.
