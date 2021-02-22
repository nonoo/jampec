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

	imgSize image.Point

	trackerRectColor color.RGBA

	selectedRect          image.Rectangle
	selectedRectColor     color.RGBA
	selectedRectSelecting bool

	reinitTrackerChan chan *image.Rectangle
}

type trackData struct {
	img  gocv.Mat
	rect image.Rectangle
}

func (s *camStruct) onMouseClick(event gocv.MouseEventType, x, y int, flags gocv.MouseEventFlag) {
	switch event {
	case gocv.MouseEventLeftButtonDown:
		s.selectedRectSelecting = true
		s.selectedRect.Min.X = x
		s.selectedRect.Min.Y = y
	case gocv.MouseEventLeftButtonUp:
		if s.selectedRectSelecting {
			s.selectedRect.Max.X = x
			s.selectedRect.Max.Y = y
			s.selectedRect = s.selectedRect.Canon()

			if !s.selectedRect.Empty() {
				if s.selectedRect.Max.X > s.imgSize.X {
					s.selectedRect.Max.X = s.imgSize.X
				}
				if s.selectedRect.Max.Y > s.imgSize.Y {
					s.selectedRect.Max.Y = s.imgSize.Y
				}
				s.reinitTrackerChan <- &s.selectedRect
			}
		}
		s.selectedRectSelecting = false
	case gocv.MouseEventMove:
		if s.selectedRectSelecting {
			s.selectedRect.Max.X = x
			s.selectedRect.Max.Y = y
		}
	case gocv.MouseEventRightButtonUp: // Cancel
		if !s.selectedRectSelecting {
			s.selectedRect = image.Rectangle{}
			s.reinitTrackerChan <- &s.selectedRect
		}
		s.selectedRectSelecting = false
	}
}

func (s *camStruct) camReadLoop(imgChan chan *gocv.Mat, errChan chan error, stopRequestedChan chan bool,
	stopFinishedChan chan bool) {

	img := gocv.NewMat()
	defer img.Close()

camReadLoop:
	for {
		select {
		case <-stopRequestedChan:
			break camReadLoop
		default:
		}

		if ok := s.cam.Read(&img); !ok {
			errChan <- errors.New("error reading camera")
			<-stopRequestedChan
			break camReadLoop
		}
		if img.Empty() {
			continue
		}

		i := img.Clone()
		select {
		case imgChan <- &i:
		case <-stopRequestedChan:
			break camReadLoop
		}
	}

	stopFinishedChan <- true
}

func (s *camStruct) trackLoop(imgToTrackChan chan *gocv.Mat, trackDataChan chan *trackData,
	errChan chan error, stopRequestedChan chan bool, stopFinishedChan chan bool) {

	var img *gocv.Mat

	img1 := gocv.NewMat()
	defer img1.Close()
	img2 := gocv.NewMat()
	defer img2.Close()
	tmpImg := gocv.NewMat()
	defer tmpImg.Close()

	var reinitTrackerRect *image.Rectangle
	var tracker gocv.Tracker
	var trackerInitialized bool

trackLoop:
	for {
		select {
		case img = <-imgToTrackChan:
		case reinitTrackerRect = <-s.reinitTrackerChan:
			continue
		case <-stopRequestedChan:
			break trackLoop
		}

		if config.ImageTransform.Grayscale {
			gocv.CvtColor(*img, &img2, gocv.ColorBGRAToGray)
		} else {
			img.CopyTo(&img2)
		}
		img.Close()

		if config.ImageTransform.BlurSize > 0 {
			gocv.GaussianBlur(img2, &img1, image.Point{config.ImageTransform.BlurSize, config.ImageTransform.BlurSize}, 0, 0, gocv.BorderDefault)
		} else {
			img2.CopyTo(&img1)
		}
		if config.ImageTransform.BinaryThreshold > 0 {
			gocv.Threshold(img1, &img2, 200, 255, gocv.ThresholdBinary)
		} else {
			img1.CopyTo(&img2)
		}
		if config.ImageTransform.ErodeDilate {
			tmpImg.SetTo(gocv.Scalar{})
			gocv.Erode(img2, &img1, tmpImg)
			tmpImg.SetTo(gocv.Scalar{})
			gocv.Dilate(img1, &img2, tmpImg)
		}
		if config.ImageTransform.Grayscale {
			gocv.CvtColor(img2, &img1, gocv.ColorGrayToBGR)
			img1.CopyTo(&img2)
		}

		if reinitTrackerRect != nil {
			if trackerInitialized {
				tracker.Close()
				trackerInitialized = false
			}
			if !reinitTrackerRect.Empty() {
				tracker = contrib.NewTrackerCSRT()
				if !tracker.Init(img2, *reinitTrackerRect) {
					errChan <- errors.New("can't init tracker")
					<-stopRequestedChan
					break trackLoop
				}
				trackerInitialized = true
			}
			reinitTrackerRect = nil
		}

		var trackRect image.Rectangle
		if trackerInitialized {
			trackRect, _ = tracker.Update(img2)
		}

		td := trackData{
			img:  img2.Clone(),
			rect: trackRect,
		}

		select {
		case trackDataChan <- &td:
		case <-stopRequestedChan:
			break trackLoop
		}
	}

	if img != nil {
		img.Close()
	}
	if trackerInitialized {
		tracker.Close()
	}

	stopFinishedChan <- true
}

func (s *camStruct) loop() {
	var exitError error

	camReadImgChan := make(chan *gocv.Mat, 25)
	camReadErrChan := make(chan error)
	camReadStopRequestedChan := make(chan bool)
	camReadStopFinishedChan := make(chan bool)
	go s.camReadLoop(camReadImgChan, camReadErrChan, camReadStopRequestedChan, camReadStopFinishedChan)

	trackImgChan := make(chan *gocv.Mat, 25)
	trackDataChan := make(chan *trackData)
	trackErrChan := make(chan error)
	trackStopRequestedChan := make(chan bool)
	trackStopFinishedChan := make(chan bool)
	go s.trackLoop(trackImgChan, trackDataChan, trackErrChan, trackStopRequestedChan, trackStopFinishedChan)

mainLoop:
	for {
		select {
		case exitError = <-camReadErrChan:
			break mainLoop
		case exitError = <-trackErrChan:
			break mainLoop
		default:
		}

		origImg := <-camReadImgChan

		size := origImg.Size()
		s.imgSize.X = size[1]
		s.imgSize.Y = size[0]

		trackImgChan <- origImg

		td := <-trackDataChan
		img := &td.img

		if !td.rect.Empty() {
			gocv.Rectangle(img, td.rect, s.trackerRectColor, 2)
			textSize := gocv.GetTextSize("Tracking", gocv.FontHersheyPlain, 1.2, 2)
			pt := image.Pt(td.rect.Max.X-textSize.X, td.rect.Min.Y-5)
			gocv.PutText(img, "Tracking", pt, gocv.FontHersheyPlain, 1.2, s.trackerRectColor, 2)
		}

		if s.selectedRectSelecting {
			gocv.Rectangle(img, s.selectedRect, s.selectedRectColor, 2)
		}

		s.window.IMShow(*img)
		img.Close()

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

	trackStopRequestedChan <- true
	<-trackStopFinishedChan

	s.errChan <- exitError
	<-s.stopRequestedChan
	s.stopFinishedChan <- true
}

func (s *camStruct) init() error {
	s.reinitTrackerChan = make(chan *image.Rectangle)

	var err error
	s.cam, err = gocv.VideoCaptureDevice(0)
	if err != nil {
		return errors.New("can't open video capture device 0")
	}

	s.window = gocv.NewWindow("jampec")

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
}
