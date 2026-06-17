# Extending chart-inspector

The three things you are most likely to change.

## Add a new endpoint

Endpoints follow a simple convention: a constructor that takes the shared dependency container and returns an HTTP handler. To add one:

1. Write the handler, using the dependencies it needs from the shared container (the cluster clients, the Helm client, the pluralizer).
2. Add any new dependency to that container, where everything is assembled once at startup.
3. Register the route alongside the others.
4. Annotate the handler for Swagger and regenerate the API docs.

## Extend what the tracer captures

The detail in the result is bounded by what the tracer records and by the fields of a resource entry. To capture more — for example the HTTP verb, request bodies, subresources, or cluster-scoped list calls — extend the tracer's logic for turning an API request into an entry, and add any new fields to the resource entry so they flow through to the response.

Keep in mind the "touched, not rendered" property: capturing more *detail per call* does not change *which* objects the dry-run touches.

## Change the dry-run behavior

The dry-run is configured where the handler builds the install request. Common changes:

- **Dry-run mode** — server-side (the default) consults the live cluster for lookups, validation, and capability discovery; a client-only mode renders locally but loses everything the live lookups would surface.
- **CRD handling** — whether CRDs are included in the render.
- **TLS** — whether to skip verification for the chart source.

Because the real machinery lives in the shared Helm library, deeper changes (caching, download behavior, the Helm action wiring) belong there, not in this service.
