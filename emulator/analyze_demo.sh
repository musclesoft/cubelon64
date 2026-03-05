#!/bin/bash
# Run emulator and track when screen memory changes significantly

cd /Users/dave/cubelon64/emulator

# Build
go build -o cubelon_emu cmd/main.go

# Run and capture interesting frames
echo "Running demo for 10000 frames (~3.3 minutes)..."
./cubelon_emu ../original/cubelon_final.tap 2>&1 | tee demo_run.log

# Analyze when screens changed
echo ""
echo "=== Screen Change Analysis ==="
grep "Saved frame" demo_run.log | grep "changed=true" | head -20
echo ""
echo "Total frames with changes:"
grep "Saved frame" demo_run.log | grep "changed=true" | wc -l

# Check frame files
echo ""
echo "=== Generated Frames ==="
ls -lt frames/frame_*.png | head -20

# Check bank contents at different times
echo ""
echo "=== Bank 5 Activity Over Time ==="
grep "Bank 5 screen area" demo_run.log
