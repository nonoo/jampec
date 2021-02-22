package main

import (
	"encoding/json"
	"os"
)

type DevConfig struct {
	Disabled       bool `json:"disabled"`
	WindowWidth    int  `json:"windowWidth"`
	WindowHeight   int  `json:"windowHeight"`
	ImageTransform struct {
		Grayscale       bool `json:"grayscale"`
		BlurSize        int  `json:"blurSize"`
		BinaryThreshold int  `json:"binaryThreshold"`
		ErodeDilate     bool `json:"erodeDilate"`
	} `json:"imageTransform"`
}

var configs []DevConfig

func loadConfig(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	defer f.Close()

	err = json.NewDecoder(f).Decode(&configs)

	if err != nil {
		return err
	}

	// Checking some needed values.
	for i := range configs {
		if configs[i].WindowWidth == 0 {
			configs[i].WindowWidth = 1280
		}
		if configs[i].WindowHeight == 0 {
			configs[i].WindowHeight = 720
		}
	}

	return nil
}
