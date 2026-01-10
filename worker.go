package main

import (
	"encoding/json"
	"log"
	"regexp"
	"sync"
	
	"github.com/TheAlyxGreen/firefly"
)

func StartDispatcher(numWorkers int, jobQueue <-chan *firefly.FirehoseEvent, broadcast chan<- []byte, textPatterns []*regexp.Regexp, urlPatterns []*regexp.Regexp) {
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(jobQueue, broadcast, textPatterns, urlPatterns)
		}()
	}
	wg.Wait()
}

func worker(jobs <-chan *firefly.FirehoseEvent, broadcast chan<- []byte, textPatterns []*regexp.Regexp, urlPatterns []*regexp.Regexp) {
	for event := range jobs {
		// We only process posts, but the check is done before sending to queue usually.
		// Double check safety.
		if event.Post == nil {
			continue
		}
		
		matched := false
		
		// 1. Check Text Regexes
		for _, pattern := range textPatterns {
			if pattern.MatchString(event.Post.Text) {
				matched = true
				break
			}
		}

		// 2. Check URL Regexes (if not already matched)
		if !matched && len(urlPatterns) > 0 {
			// Check if post has external embed
			if event.Post.Embed != nil && event.Post.Embed.External != nil {
				url := event.Post.Embed.External.URL
				for _, pattern := range urlPatterns {
					if pattern.MatchString(url) {
						matched = true
						break
					}
				}
			}
		}

		if matched {
			// Serialize the full event to JSON
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("Error marshaling event: %v", err)
				continue
			}
			broadcast <- data
		}
	}
}
