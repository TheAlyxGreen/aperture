# Aperture

Aperture is a high-performance, real-time filter for the Bluesky firehose. It connects to the Jetstream firehose, filters events based on configurable **RuleSets**, and broadcasts matching events via WebSockets.

It includes a powerful **Web Client** for visualizing and debugging the stream in real-time.

## Features

*   **Real-time Filtering**: Processes the full Bluesky firehose with low latency.
*   **Parallel Processing**: Uses a worker pool to handle high throughput.
*   **RuleSets**: Define complex filtering logic using sets of rules.
    *   **AND Logic**: Within a RuleSet, all criteria must match.
    *   **OR Logic**: If any RuleSet matches, the event is broadcast.
*   **Filtering Options**:
    *   **Collections**: Filter by event type (e.g., `app.bsky.feed.post`, `app.bsky.feed.like`). Use `*` to subscribe to all collections.
    *   **Text Content**: Regex matching on post text.
    *   **Embedded URLs**: Regex matching on external links embedded in posts.
    *   **Authors**: Exact matching on DIDs (e.g., `did:plc:...`).
    *   **Target Users**: Exact matching on the DID of the user being interacted with (liked, reposted, replied to).
    *   **Embed Types**: Filter by type of content embedded (images, video, external link, quote post).
    *   **Languages**: Filter by post language (e.g., en, ja).
    *   **Reply Status**: Filter by whether the post is a reply or an original post.
*   **WebSocket Feed**: Consumes filtered events via a WebSocket connection.

## Web Client

Aperture comes with a built-in web client (`client.html`) accessible at `http://localhost:8080`. It is designed for developers to inspect the stream and debug filters.

### Client Features
*   **Rich Event Rendering**:
    *   **Posts**: Displays text, rich text facets (hashtags, mentions, links), and embedded media.
    *   **Interactions**: Visualizes Likes, Reposts, and Follows with links to the target content/user.
    *   **System Events**: Displays Identity and Account status updates.
    *   **Embeds**: Renders images (with toggle), external links, and quote posts.
*   **Client-side Filtering**:
    *   Toggle specific **RuleSets** on/off to isolate traffic without restarting the server.
    *   Retroactive filtering applies to the current event buffer.
*   **Inspection Tools**:
    *   **Show Raw JSON**: Toggle to view the full raw JSON payload for any event.
    *   **Show Images**: Toggle image loading to save bandwidth/memory.
*   **Performance Controls**:
    *   **Buffer Size**: Configure how many events to keep in memory (default 2000).
    *   **Display Limit**: Configure how many events to render in the DOM (default 100) to keep the UI snappy.
*   **Connection Controls**:
    *   **Pause/Resume**: Freeze the display to inspect an event while the background buffer continues to fill.
    *   **Connect/Disconnect**: Manually control the WebSocket connection.
*   **Session Persistence**: All client settings (filters, limits, toggles) are saved to session storage and persist across page reloads.

## API Documentation

Aperture exposes a simple HTTP and WebSocket API for clients.

### HTTP Endpoints

#### `GET /config`
Returns public configuration details.
*   **Response**:
    ```json
    {
      "bskyServer": "https://bsky.social"
    }
    ```

#### `GET /rules`
Returns the list of configured RuleSet names.
*   **Response**:
    ```json
    ["Tech News", "Specific User", "Everything"]
    ```

### WebSocket API

#### `WS /ws`
The main event stream. Connect to `ws://localhost:8080/ws` (or your configured port).

*   **Message Format**:
    Each message is a JSON object containing the raw AT Protocol event and metadata about which rules matched.
    ```json
    {
      "event": {
        "did": "did:plc:...",
        "time_us": 1234567890,
        "kind": "commit",
        "commit": {
          "collection": "app.bsky.feed.post",
          "record": { ... },
          "rkey": "..."
        }
      },
      "matchedRules": ["Rule Name 1", "Rule Name 2"]
    }
    ```

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
  "cursorOffset": 60000000,
  "rules": [
    {
      "name": "Tech News",
      "collections": ["app.bsky.feed.post"],
      "textRegexes": ["golang", "rustlang", "AI"],
      "langs": ["en"],
      "isReply": false
    },
    {
      "name": "YouTube Links",
      "collections": ["app.bsky.feed.post"],
      "urlRegexes": ["youtube\\.com", "youtu\\.be"]
    },
    {
      "name": "Specific User",
      "collections": ["app.bsky.feed.post", "app.bsky.feed.repost"],
      "authors": ["did:plc:12345"]
    },
    {
      "name": "Interactions with User",
      "collections": ["app.bsky.feed.like", "app.bsky.feed.repost", "app.bsky.feed.post"],
      "targetUsers": ["did:plc:12345"]
    },
    {
      "name": "Art Feed",
      "collections": ["app.bsky.feed.post"],
      "embedTypes": ["images"]
    },
    {
      "name": "Account Updates",
      "collections": ["identity", "account"]
    },
    {
      "name": "Everything",
      "collections": ["*"]
    }
  ],
  "port": 8080
}
```

### Configuration Options

*   `bskyServer`: The Bluesky API endpoint (used for resolving blobs/links).
*   `jetstreamServer`: The Jetstream firehose WebSocket endpoint. Leave empty to let Firefly pick a random server.
*   `cursorOffset`: Time in microseconds to look back when starting the stream (e.g. `60000000` for 1 minute).
*   `port`: The port for the HTTP and WebSocket server.
*   `rules`: An array of **RuleSet** objects.

### RuleSet Structure

A RuleSet matches an event if **ALL** specified criteria in the set are met.

*   `name`: A friendly name for the rule (displayed in the client).
*   `collections`: List of event collections to listen for (e.g., `app.bsky.feed.post`, `app.bsky.feed.like`). Use `*` to subscribe to ALL collections. **Important:** You must specify collections here to ensure the application subscribes to them. If omitted, the rule will only match events that *other* rules have caused the app to subscribe to.
*   `textRegexes`: List of regex patterns to match against post text. (Only applies to Posts).
*   `urlRegexes`: List of regex patterns to match against embedded external URLs. (Only applies to Posts).
*   `authors`: List of exact DIDs (e.g., `did:plc:...`) to match.
*   `targetUsers`: List of exact DIDs to match as the target of an interaction (e.g. the user being liked, reposted, or replied to).
*   `embedTypes`: List of embed types to match. Values: `images`, `video`, `external`, `record` (quote post). (Only applies to Posts).
*   `langs`: List of language codes to match (e.g., `en`, `ja`). Matches if the post contains ANY of the specified languages. (Only applies to Posts).
*   `isReply`: Boolean. `true` matches only replies. `false` matches only original posts (and quote posts). If omitted, matches both. (Only applies to Posts).

## Usage

1.  Ensure the `firefly` library is available.
2.  Run the application:

```bash
go run .
```

3.  **Web Client**: Open `http://localhost:8080` in your browser.
4.  **WebSocket API**: Connect to `ws://localhost:8080/ws`.

## Architecture

*   **Ingestion**: Connects to the Bluesky firehose using the Firefly library.
*   **Worker Pool**: A pool of goroutines processes incoming events in parallel.
*   **Hub**: Manages WebSocket connections and broadcasts matching events.

## License

[MIT](LICENSE)
