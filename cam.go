package main

import (
	"errors"
	"image"
	"image/color"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

type camStruct struct {
	errChan           chan error
	stopRequestedChan chan bool
	stopFinishedChan  chan bool

	cam    *gocv.VideoCapture
	window *gocv.Window

	img1    gocv.Mat
	img2    gocv.Mat
	tmpImg  gocv.Mat
	imgSize image.Point

	tracker            gocv.Tracker
	trackerInitialized bool
	trackerRectColor   color.RGBA

	selectedRect          image.Rectangle
	selectedRectColor     color.RGBA
	selectedRectSelecting bool
	selectedRectAvailable bool

	reinitTrackerChan chan image.Rectangle
}

func (s *camStruct) onMouseClick(event gocv.MouseEventType, x, y int, flags gocv.MouseEventFlag) {
	switch event {
	case gocv.MouseEventLeftButtonDown:
		s.selectedRectSelecting = true
		s.selectedRect.Min.X = x
		s.selectedRect.Min.Y = y
	case gocv.MouseEventLeftButtonUp:
		s.selectedRect.Max.X = x
		s.selectedRect.Max.Y = y
		s.selectedRect = s.selectedRect.Canon()

		if s.selectedRect.Max.X > s.imgSize.X {
			s.selectedRect.Max.X = s.imgSize.X
		}
		if s.selectedRect.Max.Y > s.imgSize.Y {
			s.selectedRect.Max.Y = s.imgSize.Y
		}

		// Cancelled?
		if !s.selectedRectAvailable || s.selectedRect.Dx() == 0 || s.selectedRect.Dy() == 0 {
			s.selectedRectSelecting = false
			s.selectedRectAvailable = false
			break
		}

		s.selectedRectSelecting = false
		s.selectedRectAvailable = false
		s.reinitTrackerChan <- s.selectedRect
	case gocv.MouseEventMove:
		if s.selectedRectSelecting {
			s.selectedRect.Max.X = x
			s.selectedRect.Max.Y = y
			s.selectedRectAvailable = true
		}
	case gocv.MouseEventRightButtonUp: // Cancel
		s.selectedRectSelecting = false
		s.selectedRectAvailable = false
	}
}

func (s *camStruct) camReadLoop(imgChan chan *gocv.Mat, errChan chan error, stopRequestedChan chan bool,
	stopFinishedChan chan bool) {

	img := gocv.NewMat()

	for {
		select {
		case <-stopRequestedChan:
			stopFinishedChan <- true
			return
		default:
		}

		if ok := s.cam.Read(&img); !ok {
			errChan <- errors.New("error reading camera")
			<-stopRequestedChan
			stopFinishedChan <- true
			return
		}
		if img.Empty() {
			continue
		}

		i := img.Clone()
		imgChan <- &i
	}
}

func (s *camStruct) loop() {
	var exitError error

	camReadImgChan := make(chan *gocv.Mat, 25)
	camReadErrChan := make(chan error)
	camReadStopRequestedChan := make(chan bool)
	camReadStopFinishedChan := make(chan bool)

	go s.camReadLoop(camReadImgChan, camReadErrChan, camReadStopRequestedChan, camReadStopFinishedChan)

mainLoop:
	for {
		select {
		case exitError = <-camReadErrChan:
			break mainLoop
		default:
		}

		origImg := <-camReadImgChan

		size := origImg.Size()
		s.imgSize.X = size[1]
		s.imgSize.Y = size[0]

		if config.ImageTransform.Grayscale {
			gocv.CvtColor(*origImg, &s.img2, gocv.ColorBGRAToGray)
		} else {
			origImg.CopyTo(&s.img2)
		}
		origImg.Close()

		if config.ImageTransform.BlurSize > 0 {
			gocv.GaussianBlur(s.img2, &s.img1, image.Point{config.ImageTransform.BlurSize, config.ImageTransform.BlurSize}, 0, 0, gocv.BorderDefault)
		} else {
			s.img2.CopyTo(&s.img1)
		}
		if config.ImageTransform.BinaryThreshold > 0 {
			gocv.Threshold(s.img1, &s.img2, 200, 255, gocv.ThresholdBinary)
		} else {
			s.img1.CopyTo(&s.img2)
		}
		if config.ImageTransform.ErodeDilate {
			s.tmpImg.SetTo(gocv.Scalar{})
			gocv.Erode(s.img2, &s.img1, s.tmpImg)
			s.tmpImg.SetTo(gocv.Scalar{})
			gocv.Dilate(s.img1, &s.img2, s.tmpImg)
		}
		if config.ImageTransform.Grayscale {
			gocv.CvtColor(s.img2, &s.img1, gocv.ColorGrayToBGR)
			s.img1.CopyTo(&s.img2)
		}

		select {
		case r := <-s.reinitTrackerChan:
			if s.trackerInitialized {
				s.tracker.Close()
			}
			s.tracker = contrib.NewTrackerCSRT()
			if !s.tracker.Init(s.img2, r) {
				exitError = errors.New("can't init tracker")
				break mainLoop
			}
			s.trackerInitialized = true
		default:
		}

		var trackRect image.Rectangle
		if s.trackerInitialized {
			trackRect, _ = s.tracker.Update(s.img2)
		}

		if s.selectedRectAvailable {
			gocv.Rectangle(&s.img2, s.selectedRect, s.selectedRectColor, 2)
		}

		if s.trackerInitialized {
			gocv.Rectangle(&s.img2, trackRect, s.trackerRectColor, 2)
			textSize := gocv.GetTextSize("Tracking", gocv.FontHersheyPlain, 1.2, 2)
			pt := image.Pt(trackRect.Max.X-textSize.X, trackRect.Min.Y-5)
			gocv.PutText(&s.img2, "Tracking", pt, gocv.FontHersheyPlain, 1.2, s.trackerRectColor, 2)
		}

		s.window.IMShow(s.img2)

		k := s.window.WaitKey(1)
		switch k {
		case 27: // Esc
			break mainLoop
		}

		// Window closed?
		if s.window.GetWindowProperty(gocv.WindowPropertyFullscreen) < 0 {
			break mainLoop
		}
	}

	camReadStopRequestedChan <- true
	<-camReadStopFinishedChan

	s.errChan <- exitError
	<-s.stopRequestedChan
	s.stopFinishedChan <- true
}

func (s *camStruct) init() error {
	s.reinitTrackerChan = make(chan image.Rectangle, 1)

	var err error
	s.cam, err = gocv.VideoCaptureDevice(0)
	if err != nil {
		return errors.New("can't open video capture device 0")
	}

	s.window = gocv.NewWindow("jampec")

	s.img1 = gocv.NewMat()
	s.img2 = gocv.NewMat()
	s.tmpImg = gocv.NewMat()

	s.window.ResizeWindow(config.WindowWidth, config.WindowHeight)

	// Implementation from https://github.com/hybridgroup/gocv/pull/603/commits/410d1a795b55b6bbca775b5e36401c65fb05ebc5
	s.window.SetMouseCallback(s.onMouseClick)

	s.selectedRectColor = color.RGBA{255, 0, 0, 0}
	s.trackerRectColor = color.RGBA{0, 255, 0, 0}

	s.errChan = make(chan error)
	s.stopRequestedChan = make(chan bool)
	s.stopFinishedChan = make(chan bool)

	return nil
}

func (s *camStruct) deinit() {
	if s.cam != nil {
		s.cam.Close()
	}
	if s.window != nil {
		s.window.Close()
	}
	s.img1.Close()
	s.img2.Close()
	s.tmpImg.Close()
}
