package main

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/TheAlyxGreen/firefly"
)

type CompiledRuleSet struct {
	Name         string
	Collections  []string
	TextPatterns []*regexp.Regexp
	UrlPatterns  []*regexp.Regexp
	Authors      map[string]bool
	TargetUsers  map[string]bool
	EmbedTypes   []string
	Langs        []string
	IsReply      *bool
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
		var targetUserDID string

		// Try to get data from RawCommit first
		if event.RawCommit != nil {
			authorDID = event.RawCommit.Did
			if event.RawCommit.Commit != nil {
				collection = event.RawCommit.Commit.Collection
			} else if event.RawCommit.Identity != nil {
				collection = "identity"
			} else if event.RawCommit.Account != nil {
				collection = "account"
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

		// Determine Target User based on event type
		// Note: We need to extract the DID from the URI (at://did:plc:123/...)
		extractDID := func(uri string) string {
			if strings.HasPrefix(uri, "at://") {
				parts := strings.Split(uri, "/")
				if len(parts) >= 3 {
					return parts[2]
				}
			}
			return ""
		}

		switch event.Type {
		case firefly.EventTypeLike:
			if event.LikeEvent != nil {
				targetUserDID = extractDID(event.LikeEvent.Subject.Uri)
			}
		case firefly.EventTypeRepost:
			if event.RepostEvent != nil {
				targetUserDID = extractDID(event.RepostEvent.Subject.Uri)
			}
		case firefly.EventTypePost:
			if event.Post != nil && event.Post.ReplyInfo != nil {
				// For replies, the target is the parent post's author
				// Note: Firefly uses ReplyTarget, not Parent, for the immediate reply target
				targetUserDID = extractDID(event.Post.ReplyInfo.ReplyTarget.Uri)
			}
			// Add Follow support if Firefly exposes it in a friendly way, otherwise we'd need to parse RawCommit record
		}

		var matchedRules []string

		for _, rule := range rules {
			// 1. Check Collection
			if len(rule.Collections) > 0 {
				// Check for wildcard
				wildcard := false
				for _, c := range rule.Collections {
					if c == "*" {
						wildcard = true
						break
					}
				}

				if !wildcard {
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
			}

			// 2. Check Author (Exact Match)
			if len(rule.Authors) > 0 {
				if !rule.Authors[authorDID] {
					continue
				}
			}

			// 3. Check Target User (Exact Match)
			if len(rule.TargetUsers) > 0 {
				if targetUserDID == "" || !rule.TargetUsers[targetUserDID] {
					continue
				}
			}

			// 4. Check Text Patterns (if any)
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

			// 5. Check URL Patterns (if any)
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

			// 6. Check Embed Types (if any)
			if len(rule.EmbedTypes) > 0 {
				if event.Post == nil {
					continue
				}

				embedMatch := false
				var currentType string
				if event.Post.Embed != nil {
					switch event.Post.Embed.Type {
					case firefly.EmbedTypeImages:
						currentType = "images"
					case firefly.EmbedTypeVideo:
						currentType = "video"
					case firefly.EmbedTypeExternal:
						currentType = "external"
					case firefly.EmbedTypeRecord:
						currentType = "record"
					}
				}

				for _, t := range rule.EmbedTypes {
					if t == currentType {
						embedMatch = true
						break
					}
				}
				if !embedMatch {
					continue
				}
			}

			// 7. Check Languages (if any)
			if len(rule.Langs) > 0 {
				if event.Post == nil {
					continue
				}

				langMatch := false
				for _, postLang := range event.Post.Languages {
					for _, ruleLang := range rule.Langs {
						if postLang == ruleLang {
							langMatch = true
							break
						}
					}
					if langMatch {
						break
					}
				}
				if !langMatch {
					continue
				}
			}

			// 8. Check IsReply
			if rule.IsReply != nil {
				if event.Post == nil {
					continue
				}

				isReply := event.Post.ReplyInfo != nil
				if *rule.IsReply != isReply {
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
