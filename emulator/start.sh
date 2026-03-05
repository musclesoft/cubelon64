#!/bin/bash
cd "$(dirname "$0")/web"
echo "Starting CUBELON Reference Emulator..."
echo "Open: http://localhost:8888/cubelon_reference.html"
echo ""
python3 -m http.server 8888
