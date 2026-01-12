package main

import (
	"encoding/json"
	"log"
	"regexp"
	"sync"

	"github.com/TheAlyxGreen/firefly"
)

type CompiledRuleSet struct {
	Name         string
	Collections  []string
	TextPatterns []*regexp.Regexp
	UrlPatterns  []*regexp.Regexp
	Authors      map[string]bool // Set of DIDs for exact match
}

type BroadcastMessage struct {
	Event        interface{} `json:"event"` // Sending RawCommit (models.Event)
	MatchedRules []string    `json:"matchedRules"`
	AuthorHandle string      `json:"authorHandle,omitempty"` // Enriched handle if available
}

func StartDispatcher(numWorkers int, jobQueue <-chan *firefly.FirehoseEvent, broadcast chan<- []byte, rules []CompiledRuleSet) {
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(jobQueue, broadcast, rules)
		}()
	}
	wg.Wait()
}

func worker(jobs <-chan *firefly.FirehoseEvent, broadcast chan<- []byte, rules []CompiledRuleSet) {
	for event := range jobs {
		// Determine the collection of the event
		var collection string
		var authorDID string
		var authorHandle string

		// Try to get data from RawCommit first
		if event.RawCommit != nil {
			authorDID = event.RawCommit.Did
			if event.RawCommit.Commit != nil {
				collection = event.RawCommit.Commit.Collection
			}
		}

		// Fallback / Fill gaps from friendly event
		if authorDID == "" {
			authorDID = event.Repo
		}

		// Try to get handle if available (friendly event might have it)
		if event.Post != nil && event.Post.Author != nil {
			authorHandle = event.Post.Author.Handle
		} else if event.User != nil {
			authorHandle = event.User.Handle
		}

		if collection == "" {
			switch event.Type {
			case firefly.EventTypePost:
				collection = "app.bsky.feed.post"
			case firefly.EventTypeLike:
				collection = "app.bsky.feed.like"
			case firefly.EventTypeRepost:
				collection = "app.bsky.feed.repost"
			}
		}

		var matchedRules []string

		for _, rule := range rules {
			// 1. Check Collection
			if len(rule.Collections) > 0 {
				collectionMatch := false
				for _, c := range rule.Collections {
					if c == collection {
						collectionMatch = true
						break
					}
				}
				if !collectionMatch {
					continue
				}
			}

			// 2. Check Author (Exact Match)
			if len(rule.Authors) > 0 {
				if !rule.Authors[authorDID] {
					continue
				}
			}

			// 3. Check Text Patterns (if any)
			if len(rule.TextPatterns) > 0 {
				if event.Post == nil {
					continue
				}

				textConditionMet := false
				for _, pattern := range rule.TextPatterns {
					if pattern.MatchString(event.Post.Text) {
						textConditionMet = true
						break
					}
				}
				if !textConditionMet {
					continue
				}
			}

			// 4. Check URL Patterns (if any)
			if len(rule.UrlPatterns) > 0 {
				if event.Post == nil {
					continue
				}

				urlConditionMet := false
				if event.Post.Embed != nil && event.Post.Embed.External != nil {
					url := event.Post.Embed.External.URL
					for _, pattern := range rule.UrlPatterns {
						if pattern.MatchString(url) {
							urlConditionMet = true
							break
						}
					}
				}
				if !urlConditionMet {
					continue
				}
			}

			// Rule matched
			matchedRules = append(matchedRules, rule.Name)
		}

		if len(matchedRules) > 0 {
			// Use RawCommit if available, otherwise fallback to the event itself
			var payload interface{} = event.RawCommit
			if payload == nil {
				payload = event
			}

			msg := BroadcastMessage{
				Event:        payload,
				MatchedRules: matchedRules,
				AuthorHandle: authorHandle,
			}

			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Error marshaling broadcast message: %v", err)
				continue
			}
			broadcast <- data
		}
	}
}
