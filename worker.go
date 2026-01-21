package main

import (
	"encoding/json"
	"log"
	"regexp"
	"sync"
	"sync/atomic"

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
}

// RuleStats tracks the number of matches for each rule
type RuleStats struct {
	counts sync.Map // map[string]*int64
}

var GlobalRuleStats = &RuleStats{}

func (rs *RuleStats) Increment(ruleName string) {
	val, _ := rs.counts.LoadOrStore(ruleName, new(int64))
	atomic.AddInt64(val.(*int64), 1)
}

func (rs *RuleStats) GetCounts() map[string]int64 {
	result := make(map[string]int64)
	rs.counts.Range(func(key, value interface{}) bool {
		result[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	return result
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
		var targetUserDID string

		// 1. Determine Author
		authorDID = event.Repo

		// 2. Determine Collection
		switch event.Type {
		case firefly.EventTypePost:
			collection = "app.bsky.feed.post"
		case firefly.EventTypeLike:
			collection = "app.bsky.feed.like"
		case firefly.EventTypeRepost:
			collection = "app.bsky.feed.repost"
		case firefly.EventTypeDelete:
			if event.DeleteEvent != nil {
				collection = event.DeleteEvent.Collection
			}
		case firefly.EventTypeIdentity:
			collection = "identity"
		case firefly.EventTypeAccount:
			collection = "account"
		}

		// 3. Determine Target User
		getDID := func(uri string) string {
			did, err := firefly.ExtractDidFromUri(uri)
			if err != nil && err != firefly.ErrNoDid {
				return ""
			}
			return did
		}

		if event.LikeEvent != nil && event.LikeEvent.Subject != nil {
			targetUserDID = getDID(event.LikeEvent.Subject.URI)
		} else if event.RepostEvent != nil && event.RepostEvent.Subject != nil {
			targetUserDID = getDID(event.RepostEvent.Subject.URI)
		} else if event.Post != nil && event.Post.ReplyInfo != nil {
			targetUserDID = getDID(event.Post.ReplyInfo.ReplyTarget.URI)
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
				if event.Post.Embed != nil {
					for _, t := range rule.EmbedTypes {
						if t == "images" && len(event.Post.Embed.Images) > 0 {
							embedMatch = true
							break
						}
						if t == "video" && event.Post.Embed.Video != nil {
							embedMatch = true
							break
						}
						if t == "external" && event.Post.Embed.External != nil {
							embedMatch = true
							break
						}
						if t == "record" && event.Post.Embed.Record != nil {
							embedMatch = true
							break
						}
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
			GlobalRuleStats.Increment(rule.Name)
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
