# Aperture

Aperture is a high-performance, real-time filter for the Bluesky firehose. It connects to the Jetstream firehose, filters events based on configurable **RuleSets**, and broadcasts matching events via WebSockets.

## Features

*   **Real-time Filtering**: Processes the full Bluesky firehose with low latency.
*   **Parallel Processing**: Uses a worker pool to handle high throughput.
*   **RuleSets**: Define complex filtering logic using sets of rules.
    *   **AND Logic**: Within a RuleSet, all criteria must match.
    *   **OR Logic**: If any RuleSet matches, the event is broadcast.
*   **Filtering Options**:
    *   **Collections**: Filter by event type (e.g., `app.bsky.feed.post`, `app.bsky.feed.like`).
    *   **Text Content**: Regex matching on post text.
    *   **Embedded URLs**: Regex matching on external links embedded in posts.
    *   **Authors**: Exact matching on DIDs or Handles (if available).
*   **WebSocket Feed**: Consumes filtered events via a WebSocket connection.
*   **Web Client**: Includes a rich HTML client with client-side rule filtering.

## Prerequisites

*   Go 1.24 or higher
*   [Firefly](https://github.com/TheAlyxGreen/firefly) library (currently configured as a local replacement in `go.mod`)

## Configuration

Create a `config.json` file in the root directory.

### Example Configuration

```json
{
  "bskyServer": "https://bsky.social",
  "jetstreamServer": "",
  "rules": [
    {
      "name": "Tech News",
      "collections": ["app.bsky.feed.post"],
      "textRegexes": ["golang", "rustlang", "AI"]
    },
    {
      "name": "YouTube Links",
      "collections": ["app.bsky.feed.post"],
      "urlRegexes": ["youtube\\.com", "youtu\\.be"]
    },
    {
      "name": "Specific User",
      "authors": ["did:plc:12345", "alice.bsky.social"]
    }
  ],
  "port": 8080
}
```

### Configuration Options

*   `bskyServer`: The Bluesky API endpoint.
*   `jetstreamServer`: The Jetstream firehose WebSocket endpoint. Leave empty to let Firefly pick a random server.
*   `port`: The port for the HTTP and WebSocket server.
*   `rules`: An array of **RuleSet** objects.

### RuleSet Structure

A RuleSet matches an event if **ALL** specified criteria in the set are met.

*   `name`: A friendly name for the rule (displayed in the client).
*   `collections`: List of event collections to listen for (e.g., `app.bsky.feed.post`, `app.bsky.feed.like`). If omitted, defaults to all subscribed collections (but regexes might fail on non-posts).
*   `textRegexes`: List of regex patterns to match against post text. (Only applies to Posts).
*   `urlRegexes`: List of regex patterns to match against embedded external URLs. (Only applies to Posts).
*   `authors`: List of exact DIDs (e.g., `did:plc:...`) or Handles (e.g., `user.bsky.social`) to match. *Note: Handle matching depends on the event containing the handle, which is not guaranteed for all events.*

## Usage

1.  Ensure the `firefly` library is available.
2.  Run the application:

```bash
go run .
```

3.  **Web Client**: Open `http://localhost:8080` in your browser.
    *   Use the sidebar to filter the feed by specific RuleSets.
4.  **WebSocket API**: Connect to `ws://localhost:8080/ws`.
    *   **Endpoint**: `GET /rules` returns the list of configured rule names.
    *   **Message Format**:
        ```json
        {
          "event": { ... raw ATProto record ... },
          "matchedRules": ["Rule Name 1", "Rule Name 2"],
          "authorHandle": "alice.bsky.social"
        }
        ```

## Architecture

*   **Ingestion**: Connects to the Bluesky firehose using the Firefly library.
*   **Worker Pool**: A pool of goroutines processes incoming events in parallel.
*   **Hub**: Manages WebSocket connections and broadcasts matching events.

## License

[MIT](LICENSE)
