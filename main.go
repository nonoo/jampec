package main

import (
	"os"
)

func main() {
	log.Init()

	if err := loadConfig("config.json"); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	errChan := make(chan error, len(configs))
	var cams []camStruct
	for i := range configs {
		if configs[i].Disabled {
			continue
		}
		newCam := camStruct{}
		err := newCam.init(errChan, configs[i])
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}

		go newCam.loop()
		cams = append(cams, newCam)
	}

	err := <-errChan
	for i := range cams {
		cams[i].stopRequestedChan <- true
		<-cams[i].stopFinishedChan
	}

	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
