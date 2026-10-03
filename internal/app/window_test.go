package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestFitWindowFrame(t *testing.T) {
	for _, test := range []struct {
		name              string
		frame, area, want application.Rect
	}{
		{"first launch", application.Rect{}, application.Rect{X: 0, Y: 25, Width: 1440, Height: 875}, application.Rect{X: 120, Y: 91, Width: 1200, Height: 743}},
		{"first launch on small display", application.Rect{}, application.Rect{X: 0, Y: 25, Width: 800, Height: 568}, application.Rect{X: 60, Y: 68, Width: 680, Height: 482}},
		{"saved frame", application.Rect{X: 100, Y: 80, Width: 1000, Height: 700}, application.Rect{Width: 1440, Height: 900}, application.Rect{X: 100, Y: 80, Width: 1000, Height: 700}},
		{"saved compact frame", application.Rect{X: 610, Y: 348, Width: 692, Height: 512}, application.Rect{Width: 1920, Height: 1050}, application.Rect{X: 610, Y: 348, Width: 692, Height: 512}},
		{"disconnected display", application.Rect{X: -1600, Y: 80, Width: 1000, Height: 700}, application.Rect{X: 0, Y: 25, Width: 1440, Height: 875}, application.Rect{X: 0, Y: 80, Width: 1000, Height: 700}},
		{"smaller display", application.Rect{X: 500, Y: 400, Width: 1400, Height: 1000}, application.Rect{X: 0, Y: 25, Width: 900, Height: 650}, application.Rect{X: 0, Y: 25, Width: 900, Height: 650}},
		{"negative display origin", application.Rect{X: -1300, Y: -850, Width: 900, Height: 600}, application.Rect{X: -1440, Y: -900, Width: 1440, Height: 875}, application.Rect{X: -1300, Y: -850, Width: 900, Height: 600}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := fitWindowFrame(test.frame, test.area); got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestFitWindowSizeIgnoresSavedPosition(t *testing.T) {
	saved := application.Rect{X: -1600, Y: 80, Width: 1000, Height: 700}
	area := application.Rect{X: 0, Y: 25, Width: 1440, Height: 875}

	if got, want := fitWindowSize(saved, area), (application.Rect{Width: 1000, Height: 700}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFitWindowSizeUsesFirstLaunchDefaults(t *testing.T) {
	area := application.Rect{X: 0, Y: 25, Width: 1440, Height: 875}

	if got, want := fitWindowSize(application.Rect{}, area), (application.Rect{Width: 1200, Height: 743}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWriteWindowFramePersistsPathWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile with spaces", "window.json")
	first := application.Rect{X: 20, Y: 30, Width: 800, Height: 600}
	second := application.Rect{X: 40, Y: 50, Width: 700, Height: 500}
	if err := writeWindowFrame(path, first); err != nil {
		t.Fatal(err)
	}
	if err := writeWindowFrame(path, second); err != nil {
		t.Fatal(err)
	}
	if got := readWindowFrame(path); got != second {
		t.Fatalf("saved frame = %+v, want %+v", got, second)
	}
}

func TestWriteWindowFrameConcurrentWriters(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "window.json")
	const writerCount = 32

	start := make(chan struct{})
	errors := make(chan error, writerCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(writerCount)
	for index := 0; index < writerCount; index++ {
		index := index
		go func() {
			defer waitGroup.Done()
			<-start
			errors <- writeWindowFrame(path, application.Rect{
				X:      index,
				Y:      index * 2,
				Width:  800 + index,
				Height: 600 + index,
			})
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var frame application.Rect
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("final window frame is invalid JSON: %v", err)
	}
	if frame.Width < 800 || frame.Width >= 800+writerCount {
		t.Fatalf("final window frame = %+v", frame)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(directory, ".window.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary window files remain: %v", temporaryFiles)
	}
}
