#!/bin/bash
# Trace a single frame in extreme detail

echo "Tracing frame 172 in detail..."

cd ../emulator

# Run with detailed tracing
go run run_cubelon.go motion_analyzer.go assertions.go \
  --trace-music=172-172 \
  --no-frames \
  --no-audio \
  --stop-at=175 2>&1 | grep -v "Frame.*completed"

echo ""
echo "Check music_trace.txt for detailed output"
