package main

import (
	"encoding/json"
	"os"
)

type Config struct {
	WindowWidth    int `json:"windowWidth"`
	WindowHeight   int `json:"windowHeight"`
	ImageTransform struct {
		Grayscale       bool `json:"grayscale"`
		BlurSize        int  `json:"blurSize"`
		BinaryThreshold int  `json:"binaryThreshold"`
		ErodeDilate     bool `json:"erodeDilate"`
	} `json:"imageTransform"`
}

var config Config

func loadConfig(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	defer f.Close()

	err = json.NewDecoder(f).Decode(&config)

	if err != nil {
		return err
	}

	if config.WindowWidth == 0 {
		config.WindowWidth = 1280
	}
	if config.WindowHeight == 0 {
		config.WindowHeight = 720
	}
	return nil
}
