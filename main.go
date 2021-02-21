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

	cam := camStruct{}
	err := cam.init()

	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	go cam.loop()

	err = <-cam.errChan
	cam.stopRequestedChan <- true
	<-cam.stopFinishedChan

	cam.deinit()

	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
