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

	errChan := make(chan error)
	var cams []camStruct
	for i := range configs {
		if configs[i].Disabled {
			continue
		}
		newCam := camStruct{}
		err := newCam.init(errChan, i, configs[i])
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}

		go newCam.loop()
		cams = append(cams, newCam)
	}

	err := <-errChan
	for _, cam := range cams {
		cam.stopRequestedChan <- true
		<-cam.stopFinishedChan
		cam.deinit()
	}

	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
