package main

import (
	"encoding/json"
	"os"
)

type Config struct {
	BskyServer      string   `json:"bskyServer"`
	JetstreamServer string   `json:"jetstreamServer"`
	Regexes         []string `json:"regexes"`
	UrlRegexes      []string `json:"urlRegexes"`
	Port            int      `json:"port"`
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
