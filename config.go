package main

import (
	"encoding/json"
	"os"
)

type RuleSet struct {
	Name        string   `json:"name"`
	Collections []string `json:"collections"`
	TextRegexes []string `json:"textRegexes"`
	UrlRegexes  []string `json:"urlRegexes"`
	Authors     []string `json:"authors"`
	EmbedTypes  []string `json:"embedTypes"`
	Langs       []string `json:"langs"`
	IsReply     *bool    `json:"isReply,omitempty"` // true: must be reply, false: must not be reply, nil: ignore
}

type Config struct {
	BskyServer      string    `json:"bskyServer"`
	JetstreamServer string    `json:"jetstreamServer"`
	Rules           []RuleSet `json:"rules"`
	Port            int       `json:"port"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
