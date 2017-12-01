package main

import (
	"image"
	"log"
	"time"

	"github.com/andyleap/framebuffer"
	"github.com/andyleap/frameui"

	"github.com/jteeuwen/evdev"
)

func main() {
	fb, err := framebuffer.New("/dev/fb0")
	if err != nil {
		panic(err)
	}
	mainRect := image.Rect(0, 0, 800, 480)
	main := frameui.NewContainer(mainRect)

	btn := frameui.NewButton(image.Rect(0, 0, 80, 80), "Test")
	btn2 := frameui.NewButton(image.Rect(0, 400, 80, 480), "Test")
	btnQuit := frameui.NewButton(image.Rect(720, 0, 800, 80), "Quit")

	main.Add(btn)
	btn.OnClick = func(e frameui.Event) {
		btn.Text = "clicked"
		btn2.Text = "NOT"
	}
	main.Add(btn2)
	btn2.OnClick = func(e frameui.Event) {
		btn2.Text = "clicked"
		btn.Text = "NOT"
	}
	quit := false
	main.Add(btnQuit)
	btnQuit.OnClick = func(e frameui.Event) {
		quit = true
	}

	defer fb.Close()
	fb.Clear()
	fb.Swap()
	fb.Clear()
	fb.Swap()

	dev, _ := evdev.Open("/dev/input/event1")

	repaint := time.NewTicker(50 * time.Millisecond)

	mx := 0
	my := 0
	button := 0

	for !quit{
		select {
		case e := <-dev.Inbox:
			if e.Type == evdev.EvAbsolute && e.Code == evdev.AbsX {
				mx = int(e.Value)
			}
			if e.Type == evdev.EvAbsolute && e.Code == evdev.AbsY {
				my = int(e.Value)
			}
			if e.Type == evdev.EvKeys && e.Code == evdev.BtnTouch {
				log.Println("btn")
				button = 1
			}
			if e.Type == evdev.EvSync && e.Code == evdev.SynReport {
				if button > 0 {
					log.Println("Click at", mx, my)
					be := &frameui.MouseClick{
						LocatedEvent: frameui.LocatedEvent{Pos: image.Point{mx, my}},
						Button:       button,
					}
					main.Down(be)
					button = 0
				}
			}

		case <-repaint.C:
			pe := &frameui.PaintEvent{
				Image: fb,
			}
			main.Down(pe)
			fb.Swap()
		}
	}
}
