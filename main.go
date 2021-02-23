package main

import (
	"os"
	"reflect"
)

type ctrlMsgType int

const (
	ctrlMsgTypeExit              = ctrlMsgType(iota) // value1: error
	ctrlMsgTypeActive                                // value1: cam nr, value2: true/false
	ctrlMsgTypeShowOriginalImage                     // value1: true/false
)

type ctrlMsg struct {
	msgType ctrlMsgType
	value1  interface{}
	value2  interface{}
}

func main() {
	log.Init()

	if err := loadConfig("config.json"); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	var cams []camStruct
	for i := range configs {
		if configs[i].Disabled {
			continue
		}
		newCam := camStruct{}
		err := newCam.init(configs[i], i)
		if err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}

		go newCam.loop()
		cams = append(cams, newCam)
	}

	cases := make([]reflect.SelectCase, len(cams))
	for i := range cams {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(cams[i].ctrlOutChan)}
	}

	for {
		_, value, _ := reflect.Select(cases)
		msg, ok := value.Interface().(ctrlMsg)
		if !ok {
			continue
		}

		switch msg.msgType {
		case ctrlMsgTypeExit:
			err, _ := msg.value1.(error)
			for i := range cams {
				cams[i].stopRequestedChan <- true
				<-cams[i].stopFinishedChan
			}

			if err != nil {
				log.Error(err.Error())
				os.Exit(1)
			}
			return
		case ctrlMsgTypeActive:
			nr := -1
			for i := range cams {
				if cams[i].config.DevNum == msg.value1 {
					nr = i
				} else {
					cams[i].ctrlInChan <- ctrlMsg{msgType: ctrlMsgTypeActive, value2: false}
				}
			}
			if nr >= 0 {
				cams[nr].ctrlInChan <- ctrlMsg{msgType: ctrlMsgTypeActive, value2: true}
			}
		case ctrlMsgTypeShowOriginalImage:
			for i := range cams {
				cams[i].ctrlInChan <- ctrlMsg{msgType: ctrlMsgTypeShowOriginalImage, value1: msg.value1}
			}
		}
	}
}
