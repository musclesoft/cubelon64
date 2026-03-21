package main

import (
	"fmt"
	"os"
)

// WatchpointTracer logs memory accesses to specific addresses
type WatchpointTracer struct {
	enabled   bool
	startFrame int
	endFrame   int
	addresses  map[uint16]string // address -> description
	logFile    *os.File
	hitCount   map[uint16]int    // address -> access count
}

func NewWatchpointTracer(startFrame, endFrame int) *WatchpointTracer {
	return &WatchpointTracer{
		enabled:    false,
		startFrame: startFrame,
		endFrame:   endFrame,
		addresses:  make(map[uint16]string),
		hitCount:   make(map[uint16]int),
	}
}

func (wt *WatchpointTracer) AddWatch(addr uint16, description string) {
	wt.addresses[addr] = description
}

func (wt *WatchpointTracer) Enable(filename string) error {
	var err error
	wt.logFile, err = os.Create(filename)
	if err != nil {
		return err
	}
	wt.enabled = true

	fmt.Fprintf(wt.logFile, "=== Memory Watchpoint Trace ===\n")
	fmt.Fprintf(wt.logFile, "Frames: %d-%d\n", wt.startFrame, wt.endFrame)
	fmt.Fprintf(wt.logFile, "Watching:\n")
	for addr, desc := range wt.addresses {
		fmt.Fprintf(wt.logFile, "  $%04X: %s\n", addr, desc)
	}
	fmt.Fprintf(wt.logFile, "\n")

	return nil
}

func (wt *WatchpointTracer) CheckFrame(frame int) bool {
	return frame >= wt.startFrame && frame <= wt.endFrame
}

func (wt *WatchpointTracer) LogRead(frame int, pc uint16, addr uint16, value byte) {
	if !wt.enabled {
		return
	}

	if desc, watched := wt.addresses[addr]; watched {
		wt.hitCount[addr]++
		fmt.Fprintf(wt.logFile, "Frame %04d: READ  $%04X = $%02X  (PC=$%04X)  // %s\n",
			frame, addr, value, pc, desc)
	}
}

func (wt *WatchpointTracer) LogWrite(frame int, pc uint16, addr uint16, value byte) {
	if !wt.enabled {
		return
	}

	if desc, watched := wt.addresses[addr]; watched {
		wt.hitCount[addr]++
		fmt.Fprintf(wt.logFile, "Frame %04d: WRITE $%04X <- $%02X  (PC=$%04X)  // %s\n",
			frame, addr, value, pc, desc)
	}
}

func (wt *WatchpointTracer) Close() {
	if wt.logFile != nil {
		// Write summary
		fmt.Fprintf(wt.logFile, "\n=== Summary ===\n")
		for addr, desc := range wt.addresses {
			count := wt.hitCount[addr]
			fmt.Fprintf(wt.logFile, "$%04X (%s): %d accesses\n", addr, desc, count)
		}

		wt.logFile.Close()
		wt.enabled = false
	}
}
