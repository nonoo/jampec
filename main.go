package main

import (
	"image"
	"image/color"
	"os"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

var imgSize image.Point

var tracker gocv.Tracker
var trackerInitialized bool

var selectedRect image.Rectangle
var selectedRectSelecting bool
var selectedRectAvailable bool

var reinitTrackerChan chan image.Rectangle = make(chan image.Rectangle, 1)

func onMouseClick(event gocv.MouseEventType, x, y int, flags gocv.MouseEventFlag) {
	switch event {
	case gocv.MouseEventLeftButtonDown:
		selectedRectSelecting = true
		selectedRect.Min.X = x
		selectedRect.Min.Y = y
	case gocv.MouseEventLeftButtonUp:
		selectedRect.Max.X = x
		selectedRect.Max.Y = y
		selectedRect = selectedRect.Canon()

		if selectedRect.Max.X > imgSize.X {
			selectedRect.Max.X = imgSize.X
		}
		if selectedRect.Max.Y > imgSize.Y {
			selectedRect.Max.Y = imgSize.Y
		}

		// Cancelled?
		if !selectedRectAvailable || selectedRect.Dx() == 0 || selectedRect.Dy() == 0 {
			selectedRectSelecting = false
			selectedRectAvailable = false
			break
		}

		selectedRectSelecting = false
		selectedRectAvailable = false
		reinitTrackerChan <- selectedRect
	case gocv.MouseEventMove:
		if selectedRectSelecting {
			selectedRect.Max.X = x
			selectedRect.Max.Y = y
			selectedRectAvailable = true
		}
	case gocv.MouseEventRightButtonUp: // Cancel
		selectedRectSelecting = false
		selectedRectAvailable = false
	}
}

func main() {
	log.Init()

	if err := loadConfig("config.json"); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		log.Error("can't open video capture device 0")
		os.Exit(1)
	}
	defer webcam.Close()

	window := gocv.NewWindow("jampec")
	defer window.Close()

	img1 := gocv.NewMat()
	defer img1.Close()

	img2 := gocv.NewMat()
	defer img2.Close()

	tmpImg := gocv.NewMat()
	defer tmpImg.Close()

	window.ResizeWindow(config.WindowWidth, config.WindowHeight)

	// Implementation from https://github.com/hybridgroup/gocv/pull/603/commits/410d1a795b55b6bbca775b5e36401c65fb05ebc5
	window.SetMouseCallback(onMouseClick)

	red := color.RGBA{255, 0, 0, 0}
	green := color.RGBA{0, 255, 0, 0}

	for {
		if ok := webcam.Read(&img1); !ok {
			log.Error("error reading camera")
			os.Exit(1)
		}
		if img1.Empty() {
			continue
		}

		s := img1.Size()
		imgSize.X = s[1]
		imgSize.Y = s[0]

		if config.ImageTransform.Grayscale {
			gocv.CvtColor(img1, &img2, gocv.ColorBGRAToGray)
		} else {
			img1.CopyTo(&img2)
		}
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

		select {
		case r := <-reinitTrackerChan:
			if trackerInitialized {
				tracker.Close()
			}
			tracker = contrib.NewTrackerCSRT()
			if !tracker.Init(img2, r) {
				log.Error("can't init tracker")
				os.Exit(1)
			}
			trackerInitialized = true
		default:
		}

		var trackRect image.Rectangle
		if trackerInitialized {
			trackRect, _ = tracker.Update(img2)
		}

		if selectedRectAvailable {
			gocv.Rectangle(&img2, selectedRect, red, 2)
		}

		if trackerInitialized {
			gocv.Rectangle(&img2, trackRect, green, 2)
			textSize := gocv.GetTextSize("Tracking", gocv.FontHersheyPlain, 1.2, 2)
			pt := image.Pt(trackRect.Max.X-textSize.X, trackRect.Min.Y-5)
			gocv.PutText(&img2, "Tracking", pt, gocv.FontHersheyPlain, 1.2, green, 2)
		}

		window.IMShow(img2)

		k := window.WaitKey(1)
		switch k {
		case 27: // Esc
			return
		}

		// Window closed?
		if window.GetWindowProperty(gocv.WindowPropertyFullscreen) < 0 {
			return
		}
	}
}
