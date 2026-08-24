# webhook-relay

A small Go service that receives one webhook and fans it out to many downstream
targets. Built while wiring n8n workflows that needed the same event delivered
to several consumers without chaining them together.

## Why

Chaining webhook consumers means the slowest one sets the pace, and a single
failure loses the event for everything downstream of it. `webhook-relay` gives
each target its own queue and its own retry budget instead.

## Usage

```sh
go run . -targets "https://a.example/hook,https://b.example/hook"
```

| Flag       | Default | Description                       |
|------------|---------|-----------------------------------|
| `-addr`    | `:8080` | Listen address                    |
| `-targets` | —       | Comma-separated downstream URLs   |
| `-workers` | `4`     | Delivery workers per target       |

`RELAY_TARGETS` is read when `-targets` is not supplied.

## Behaviour

- `POST /hook` buffers the payload (1 MiB cap) and answers `202` immediately.
- `GET /healthz` returns `ok`.
- Failed deliveries retry 5 times with exponential backoff starting at 200ms.
- `4xx` responses other than `429` are treated as permanent and not retried.
- A saturated target queue sheds its payload rather than blocking the listener.

## Tests

```sh
go test ./...
```
