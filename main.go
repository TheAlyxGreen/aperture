package main

import (
	"context"
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

func main() {
	// 1. Load Configuration
	config, err := LoadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	// 2. Compile Text Regexes
	var textRegexes []*regexp.Regexp
	for _, r := range config.Regexes {
		compiled, err := regexp.Compile(r)
		if err != nil {
			log.Fatalf("Invalid text regex '%s': %v", r, err)
		}
		textRegexes = append(textRegexes, compiled)
	}
	log.Printf("Loaded %d text regexes", len(textRegexes))

	// 3. Compile URL Regexes
	var urlRegexes []*regexp.Regexp
	for _, r := range config.UrlRegexes {
		compiled, err := regexp.Compile(r)
		if err != nil {
			log.Fatalf("Invalid url regex '%s': %v", r, err)
		}
		urlRegexes = append(urlRegexes, compiled)
	}
	log.Printf("Loaded %d url regexes", len(urlRegexes))
	
	// 4. Start the Hub
	hub := NewHub()
	go hub.Run()
	
	// 5. Setup Worker Pool
	// We need a channel to buffer incoming posts from Firefly
	jobQueue := make(chan *firefly.FirehoseEvent, 1000) // Buffer size 1000
	
	// Start workers
	go StartDispatcher(runtime.NumCPU(), jobQueue, hub.broadcast, textRegexes, urlRegexes)
	
	// 6. Start Firefly Consumer
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
			Collections: []string{"app.bsky.feed.post"},
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
			
			if event.Type == firefly.EventTypePost {
				jobQueue <- event
			}
		}
	}()
	
	// 7. Start HTTP Server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "client.html")
	})
	
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
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
}
