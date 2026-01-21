package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"runtime"
	"time"

	"github.com/TheAlyxGreen/firefly"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// PublicConfig exposes safe configuration to the client
type PublicConfig struct {
	BskyServer string `json:"bskyServer"`
}

func main() {
	// 1. Load Configuration
	config, err := LoadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Compile Rules and Aggregate Collections/Authors
	var compiledRules []CompiledRuleSet
	collectionsMap := make(map[string]bool)
	authorsMap := make(map[string]bool)
	var ruleNames []string
	subscribeToAllCollections := false
	subscribeToAllAuthors := false

	// If no rules are defined, we default to subscribing to everything (or nothing, but let's assume everything for authors)
	if len(config.Rules) == 0 {
		subscribeToAllAuthors = true
	}

	for i, rule := range config.Rules {
		var cr CompiledRuleSet
		cr.Name = rule.Name
		if cr.Name == "" {
			cr.Name = fmt.Sprintf("Rule #%d", i+1)
		}
		ruleNames = append(ruleNames, cr.Name)

		// Collections
		cr.Collections = rule.Collections
		for _, c := range rule.Collections {
			if c == "*" {
				subscribeToAllCollections = true
			}
			collectionsMap[c] = true
		}

		// Compile Text Regexes
		for _, r := range rule.TextRegexes {
			compiled, err := regexp.Compile(r)
			if err != nil {
				log.Fatalf("Invalid text regex '%s' in rule '%s': %v", r, cr.Name, err)
			}
			cr.TextPatterns = append(cr.TextPatterns, compiled)
		}

		// Compile URL Regexes
		for _, r := range rule.UrlRegexes {
			compiled, err := regexp.Compile(r)
			if err != nil {
				log.Fatalf("Invalid url regex '%s' in rule '%s': %v", r, cr.Name, err)
			}
			cr.UrlPatterns = append(cr.UrlPatterns, compiled)
		}

		// Authors (Exact Match)
		if len(rule.Authors) > 0 {
			cr.Authors = make(map[string]bool)
			for _, author := range rule.Authors {
				cr.Authors[author] = true
				authorsMap[author] = true
			}
		} else {
			// If a rule has no specific authors, it needs to listen to ALL authors
			subscribeToAllAuthors = true
		}

		// Target Users (Exact Match)
		if len(rule.TargetUsers) > 0 {
			cr.TargetUsers = make(map[string]bool)
			for _, target := range rule.TargetUsers {
				cr.TargetUsers[target] = true
			}
		}

		// Embed Types & Langs & IsReply
		cr.EmbedTypes = rule.EmbedTypes
		cr.Langs = rule.Langs
		cr.IsReply = rule.IsReply

		compiledRules = append(compiledRules, cr)
	}
	log.Printf("Loaded %d rule sets", len(compiledRules))

	// Determine Collections to subscribe to
	var collections []string
	if !subscribeToAllCollections {
		for c := range collectionsMap {
			collections = append(collections, c)
		}
		if len(collections) == 0 {
			// Default to posts if nothing specified, to avoid empty stream
			collections = []string{"app.bsky.feed.post"}
		}
		log.Printf("Subscribing to collections: %v", collections)
	} else {
		log.Printf("Subscribing to ALL collections (*)")
		collections = nil // Firefly/Jetstream convention for "all"
	}

	// Determine Authors to subscribe to
	var authors []string
	if !subscribeToAllAuthors {
		for a := range authorsMap {
			authors = append(authors, a)
		}
		log.Printf("Subscribing to %d specific authors", len(authors))
	} else {
		log.Printf("Subscribing to ALL authors")
		authors = nil
	}

	// Determine Cursor
	var cursor *int64
	if config.CursorOffset > 0 {
		c := time.Now().UnixMicro() - config.CursorOffset
		cursor = &c
		log.Printf("Starting replay from %d microseconds ago (Cursor: %d)", config.CursorOffset, *cursor)
	}

	// 3. Start the Hub
	hub := NewHub()
	go hub.Run()

	// 4. Setup Worker Pool
	// We need a channel to buffer incoming posts from Firefly
	jobQueue := make(chan *firefly.FirehoseEvent, 1000) // Buffer size 1000

	// Start workers
	go StartDispatcher(runtime.NumCPU(), jobQueue, hub.broadcast, compiledRules)

	// 5. Start Firefly Consumer
	go func() {
		log.Println("Connecting to Bluesky...")
		ctx := context.Background()

		// Create client
		client, err := firefly.NewCustomInstance(ctx, config.BskyServer, new(http.Client))
		if err != nil {
			log.Printf("Error creating firefly client: %v", err)
			return
		}

		// Determine Firehose URL
		var jetstreamURL *string
		log.Printf("StreamEvents starting...")
		if config.JetstreamServer != "" {
			jetstreamURL = &config.JetstreamServer
			log.Printf("URL: %s\n", *jetstreamURL)
		} else {
			log.Printf("URL: <default>")
		}

		events, err := client.StreamEvents(ctx, &firefly.FirehoseOptions{
			Collections: collections,
			Authors:     authors,
			Cursor:      cursor,
			BufferSize:  1000,
			URL:         jetstreamURL,
		})
		if err != nil {
			log.Printf("Error starting firehose: %v", err)
			return
		}

		count := 0
		lastLog := time.Now()

		for event := range events {
			count++
			if time.Since(lastLog) > 30*time.Second {
				log.Printf("Heartbeat: Received %d events in last 30s", count)
				count = 0
				lastLog = time.Now()
			}

			// We now pass ALL events to the worker, not just posts
			// The worker will filter based on collection
			jobQueue <- event
		}
	}()

	// 6. Start HTTP Server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "client.html")
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	http.HandleFunc("/rules", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ruleNames)
	})

	http.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PublicConfig{
			BskyServer: config.BskyServer,
		})
	})

	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("Server starting on %s", addr)
	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	hub.register <- conn

	// Start a read loop to handle control messages (Close, Ping, etc.)
	// This ensures the connection is properly maintained and closed.
	go func() {
		defer func() {
			hub.unregister <- conn
			conn.Close()
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("websocket error: %v", err)
				}
				break
			}
		}
	}()
}
