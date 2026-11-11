package cycleprofile

import (
	"fmt"
	"sort"
	"strings"
)

// Attributes the cycles of the C64 CPU, including the cycles the VIC takes from it, to the
// routine each instruction belongs to: the nearest code symbol at or below its address
type Profiler struct {
	addresses []int
	names     map[int]string
	cycles    map[string]uint64
	Frames    int
	total     uint64
}

func New(symbols map[string]int) *Profiler {
	profiler := &Profiler{names: map[int]string{}, cycles: map[string]uint64{}}
	for name, address := range symbols {
		if address < 0x0801 || strings.ToUpper(name) == name {
			continue
		}
		if at := strings.IndexByte(name, '@'); at >= 0 {
			name = name[:at]
		}
		if _, exists := profiler.names[address]; !exists {
			profiler.addresses = append(profiler.addresses, address)
		}
		profiler.names[address] = name
	}
	sort.Ints(profiler.addresses)
	return profiler
}

func (profiler *Profiler) Add(pc uint16, cycles uint64) {
	index := sort.SearchInts(profiler.addresses, int(pc)+1) - 1
	name := fmt.Sprintf("$%04X", pc)
	if index >= 0 {
		name = profiler.names[profiler.addresses[index]]
	}
	profiler.cycles[name] += cycles
	profiler.total += cycles
}

func (profiler *Profiler) Report() {
	if profiler.Frames == 0 {
		fmt.Println("Profile: no frames in the range")
		return
	}
	type row struct {
		name   string
		cycles uint64
	}
	rows := make([]row, 0, len(profiler.cycles))
	for name, cycles := range profiler.cycles {
		rows = append(rows, row{name, cycles})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].cycles > rows[j].cycles })
	fmt.Printf("Profile over %d frames, cycles per frame (a PAL frame has %d):\n", profiler.Frames, 312*63)
	for _, r := range rows {
		if r.cycles/uint64(profiler.Frames) == 0 {
			break
		}
		fmt.Printf("  %-28s %6d\n", r.name, r.cycles/uint64(profiler.Frames))
	}
}
