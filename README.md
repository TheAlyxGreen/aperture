# Aperture

Aperture is a high-performance, real-time filter for the Bluesky firehose. It connects to the Jetstream firehose, filters posts based on configurable regex patterns (text and URLs), and broadcasts matching events via WebSockets.

## Features

*   **Real-time Filtering**: Processes the full Bluesky firehose with low latency.
*   **Parallel Processing**: Uses a worker pool to handle high throughput.
*   **Flexible Configuration**: Filter by post text content or embedded URLs using regex.
*   **WebSocket Feed**: Consumes filtered events easily via a WebSocket connection.
*   **Web Client**: Includes a simple HTML client for monitoring the feed.

## Prerequisites

*   Go 1.24 or higher
*   [Firefly](https://github.com/TheAlyxGreen/firefly) library (currently configured as a local replacement in `go.mod`)

## Configuration

Create a `config.json` file in the root directory. You can use the provided example or modify it:

```json
{
  "bskyServer": "https://bsky.social",
  "jetstreamServer": "wss://bsky.network",
  "regexes": [
    "\\.com",
    "important topic"
  ],
  "urlRegexes": [
    "youtube\\.com",
    "youtu\\.be"
  ],
  "port": 8080
}
```

*   `bskyServer`: The Bluesky API endpoint.
*   `jetstreamServer`: The Jetstream firehose WebSocket endpoint (e.g., `wss://bsky.network`).
*   `regexes`: A list of regex patterns to match against the post text.
*   `urlRegexes`: A list of regex patterns to match against embedded external URLs.
*   `port`: The port for the HTTP and WebSocket server.

## Usage

1.  Ensure the `firefly` library is available (check `go.mod`).
2.  Run the application:

```bash
go run .
```

3.  Open your browser to `http://localhost:8080` to view the live feed.
4.  Connect programmatically via WebSocket at `ws://localhost:8080/ws`.

## Architecture

*   **Ingestion**: Connects to the Bluesky firehose using the Firefly library.
*   **Worker Pool**: A pool of goroutines processes incoming events in parallel, checking them against the configured regex patterns.
*   **Hub**: Manages WebSocket connections and broadcasts matching events to all connected clients.

## License

[MIT](LICENSE)
